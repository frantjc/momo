package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	_ "github.com/928799934/go-png-cgbi"
	"github.com/frantjc/momo"
	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/internal/controller"
	"github.com/frantjc/momo/internal/momoutil"
	xos "github.com/frantjc/x/os"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/s3blob"
	"golang.org/x/sync/errgroup"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	// +kubebuilder:scaffold:imports
)

func main() {
	var (
		ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		err       error
	)

	if err = NewEntrypoint().ExecuteContext(ctx); err != nil && !errors.Is(err, context.Canceled) {
		os.Stderr.WriteString(err.Error() + "\n")
		stop()
		xos.ExitFromError(err)
	}

	stop()
}

var (
	scheme = k8sruntime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(momov1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// NewEntrypoint returns the command which acts as
// the entrypoint for `momo`.
func NewEntrypoint() *cobra.Command {
	var (
		verbosity int
		cmd       = &cobra.Command{
			Use:           "momo",
			Version:       SemVer(),
			SilenceErrors: true,
			SilenceUsage:  true,
			PersistentPreRun: func(cmd *cobra.Command, _ []string) {
				var (
					log = slog.New(slog.NewTextHandler(cmd.OutOrStdout(), &slog.HandlerOptions{
						Level: slog.Level(int(slog.LevelError) - 4*verbosity),
					}))
					slogr = logr.FromSlogHandler(log.Handler())
				)

				ctrl.SetLogger(slogr)
				cmd.SetContext(logr.NewContext(cmd.Context(), slogr))
			},
		}
	)

	cmd.SetVersionTemplate("{{ .Name }}{{ .Version }} " + runtime.Version() + "\n")
	cmd.PersistentFlags().CountVarP(&verbosity, "verbose", "V", "Verbosity.")

	cmd.AddCommand(
		NewControl(),
		NewServe(),
		NewUpload(),
	)

	return cmd
}

func NewControl() *cobra.Command {
	var (
		metricsAddr                                      string
		metricsCertPath, metricsCertName, metricsCertKey string
		webhookCertPath, webhookCertName, webhookCertKey string
		enableLeaderElection                             bool
		probeAddr                                        string
		secureMetrics                                    bool
		enableHTTP2                                      bool
		cmd                                              = &cobra.Command{
			Use:           "ctrl",
			SilenceErrors: true,
			SilenceUsage:  true,
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := ctrl.GetConfig()
				if err != nil {
					return err
				}

				var (
					ctx     = cmd.Context()
					tlsOpts []func(*tls.Config)
				)

				if !enableHTTP2 {
					tlsOpts = append(tlsOpts, func(c *tls.Config) {
						c.NextProtos = []string{"http/1.1"}
					})
				}

				var (
					metricsCertWatcher *certwatcher.CertWatcher
					webhookCertWatcher *certwatcher.CertWatcher
					webhookTLSOpts     = tlsOpts
				)

				if len(webhookCertPath) > 0 {
					var err error
					webhookCertWatcher, err = certwatcher.New(
						filepath.Join(webhookCertPath, webhookCertName),
						filepath.Join(webhookCertPath, webhookCertKey),
					)
					if err != nil {
						return err
					}

					webhookTLSOpts = append(webhookTLSOpts, func(config *tls.Config) {
						config.GetCertificate = webhookCertWatcher.GetCertificate
					})
				}

				var (
					webhookServer = webhook.NewServer(webhook.Options{
						TLSOpts: webhookTLSOpts,
					})
					metricsServerOptions = metricsserver.Options{
						BindAddress:   metricsAddr,
						SecureServing: secureMetrics,
						TLSOpts:       tlsOpts,
					}
				)

				if secureMetrics {
					metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
				}

				if len(metricsCertPath) > 0 {
					var err error
					metricsCertWatcher, err = certwatcher.New(
						filepath.Join(metricsCertPath, metricsCertName),
						filepath.Join(metricsCertPath, metricsCertKey),
					)
					if err != nil {
						return err
					}

					metricsServerOptions.TLSOpts = append(metricsServerOptions.TLSOpts, func(config *tls.Config) {
						config.GetCertificate = metricsCertWatcher.GetCertificate
					})
				}

				mgr, err := ctrl.NewManager(cfg, ctrl.Options{
					Scheme:                        scheme,
					Metrics:                       metricsServerOptions,
					WebhookServer:                 webhookServer,
					HealthProbeBindAddress:        probeAddr,
					LeaderElection:                enableLeaderElection,
					LeaderElectionID:              "dfc6d68d.frantj.cc",
					LeaderElectionReleaseOnCancel: true,
				})
				if err != nil {
					return err
				}

				if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
					return err
				}

				if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
					return err
				}

				if err = (&controller.BucketReconciler{}).SetupWithManager(mgr); err != nil {
					return err
				}

				if err = (&controller.IPAReconciler{}).SetupWithManager(mgr); err != nil {
					return err
				}

				if err = (&controller.APKReconciler{}).SetupWithManager(mgr); err != nil {
					return err
				}

				if err = (&controller.MobileAppReconciler{}).SetupWithManager(mgr); err != nil {
					return err
				}

				// +kubebuilder:scaffold:builder

				if metricsCertWatcher != nil {
					if err := mgr.Add(metricsCertWatcher); err != nil {
						return err
					}
				}

				if webhookCertWatcher != nil {
					if err := mgr.Add(webhookCertWatcher); err != nil {
						return err
					}
				}

				return mgr.Start(ctx)
			},
		}
	)

	// Just allow this flag to be passed, it's parsed by ctrl.GetConfig().
	cmd.Flags().String("kubeconfig", "", "Kube config.")

	cmd.Flags().StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	cmd.Flags().StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	cmd.Flags().BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	cmd.Flags().BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	cmd.Flags().StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	cmd.Flags().StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	cmd.Flags().StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	cmd.Flags().StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	cmd.Flags().StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	cmd.Flags().StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	cmd.Flags().BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")

	return cmd
}

func NewServe() *cobra.Command {
	var (
		addr string
		cmd  = &cobra.Command{
			Use:           "srv",
			SilenceErrors: true,
			SilenceUsage:  true,
			RunE: func(cmd *cobra.Command, args []string) error {
				l, err := net.Listen("tcp", addr)
				if err != nil {
					return err
				}
				defer l.Close()

				var (
					srv = &http.Server{
						Addr:              addr,
						ReadHeaderTimeout: time.Second * 5,
						Handler:           momoutil.NewHandler(scheme),
					}
					eg, ctx = errgroup.WithContext(cmd.Context())
				)

				eg.Go(func() error {
					return srv.Serve(l)
				})

				eg.Go(func() error {
					<-ctx.Done()
					cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second*30)
					defer cancel()
					if err = srv.Shutdown(cctx); err != nil {
						return err
					}
					return ctx.Err()
				})

				return eg.Wait()
			},
		}
	)

	// Just allow this flag to be passed, it's parsed by ctrl.GetConfig().
	cmd.Flags().String("kubeconfig", "", "Kube config.")
	cmd.Flags().StringVar(&addr, "addr", ":8080", "listen address for momo")

	return cmd
}

func NewUpload() *cobra.Command {
	var (
		addr string
		cmd  = &cobra.Command{
			Use:           "upload [flags] (namespace) (bucket) (name) (.ipa|.apk)",
			Args:          cobra.ExactArgs(4),
			SilenceErrors: true,
			SilenceUsage:  true,
			RunE: func(cmd *cobra.Command, args []string) error {
				var (
					ctx        = cmd.Context()
					namespace  = args[0]
					bucketName = args[1]
					appName    = args[2]
					file       = args[3]
					cli        = new(momo.Client)
				)

				f, err := os.Open(file)
				if err != nil {
					return err
				}
				defer f.Close()

				var (
					ext         = strings.ToLower(filepath.Ext(file))
					contentType string
				)
				switch ext {
				case ".apk":
					contentType = momoutil.ContentTypeAPK
				case ".ipa":
					contentType = momoutil.ContentTypeIPA
				}

				if addr != "" {
					var err error
					if cli.BaseURL, err = url.Parse(addr); err != nil {
						return err
					}
				}

				cfg, err := ctrl.GetConfig()
				if err == nil {
					cli.HTTPClient = http.DefaultClient
					cli.HTTPClient.Transport = &kubeAuthTransport{config: cfg}
				}

				if err := cli.UploadApp(ctx, f, contentType, namespace, bucketName, appName); err != nil {
					return err
				}

				return nil
			},
		}
	)

	cmd.Flags().StringVar(&addr, "addr", "", "listen address for momo")

	return cmd
}

type kubeAuthTransport struct {
	config       *rest.Config
	roundTripper http.RoundTripper
}

func (t *kubeAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t == nil {
		return http.DefaultTransport.RoundTrip(req)
	}

	if t.roundTripper == nil {
		t.roundTripper = http.DefaultTransport
	}

	if t.config == nil {
		return t.roundTripper.RoundTrip(req)
	}

	if t.config.BearerToken != "" {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", t.config.BearerToken))
	} else if t.config.Username != "" && t.config.Password != "" {
		req.SetBasicAuth(t.config.Username, t.config.Password)
	}

	return t.roundTripper.RoundTrip(req)
}
