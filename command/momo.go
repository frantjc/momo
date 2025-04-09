package command

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	_ "github.com/928799934/go-png-cgbi"
	"github.com/frantjc/momo"
	"github.com/frantjc/momo/android"
	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/internal/api"
	"github.com/frantjc/momo/internal/controller"
	"github.com/frantjc/momo/internal/momoutil"
	"github.com/spf13/cobra"
	_ "gocloud.dev/blob/fileblob"
	_ "gocloud.dev/blob/s3blob"
	"golang.org/x/sync/errgroup"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// NewMomo returns the command which acts as
// the entrypoint for `momo`.
func NewMomo() *cobra.Command {
	var (
		cmd = &cobra.Command{Use: "momo"}
	)

	cmd.AddCommand(
		NewControl(),
		NewServe(),
		NewUnpack(),
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
			Use: "ctrl",
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

				scheme, err := momoutil.NewScheme(momov1alpha1.AddToScheme, clientgoscheme.AddToScheme)
				if err != nil {
					return err
				}

				mgr, err := ctrl.NewManager(cfg, ctrl.Options{
					Scheme:                        scheme,
					Metrics:                       metricsServerOptions,
					WebhookServer:                 webhookServer,
					HealthProbeBindAddress:        probeAddr,
					LeaderElection:                enableLeaderElection,
					LeaderElectionID:              "dfc6d68d.momo.frantj.cc",
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

	cmd.Flags().StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service")
	cmd.Flags().StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to")
	cmd.Flags().BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager")
	cmd.Flags().BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead")
	cmd.Flags().StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate")
	cmd.Flags().StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file")
	cmd.Flags().StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file")
	cmd.Flags().StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate")
	cmd.Flags().StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file")
	cmd.Flags().StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file")
	cmd.Flags().BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")

	return cmd
}

func NewServe() *cobra.Command {
	var (
		port int
		opts = &api.Opts{
			Swagger: true,
		}
		cmd = &cobra.Command{
			Use: "srv",
			RunE: func(cmd *cobra.Command, args []string) error {
				l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
				if err != nil {
					return err
				}
				defer l.Close()

				eg, ctx := errgroup.WithContext(cmd.Context())

				if len(args) > 0 {
					var ex *exec.Cmd
					opts.Fallback, ex, err = momoutil.NewExecHandlerWithPortFromEnv(ctx, args[0], args[1:]...)
					if err != nil {
						return err
					}

					// A rough algorithm for making the working directory of
					// the exec the directory of the entrypoint in the case
					// of the args being something like `node /app/server.js`.
					for _, entrypoint := range args[1:] {
						if fi, err := os.Stat(entrypoint); err == nil {
							if fi.IsDir() {
								ex.Dir = filepath.Clean(entrypoint)
							} else {
								ex.Dir = filepath.Dir(entrypoint)
							}
							break
						}
					}

					eg.Go(ex.Run)
				}

				handler, err := api.NewHandler(opts)
				if err != nil {
					return err
				}

				srv := &http.Server{
					ReadHeaderTimeout: time.Second * 5,
					Handler:           handler,
					BaseContext: func(_ net.Listener) context.Context {
						return context.WithoutCancel(cmd.Context())
					},
				}

				eg.Go(func() error {
					return srv.Serve(l)
				})

				eg.Go(func() error {
					<-ctx.Done()
					if err = srv.Shutdown(context.WithoutCancel(ctx)); err != nil {
						return err
					}
					return ctx.Err()
				})

				return eg.Wait()
			},
		}
	)

	cmd.Flags().IntVarP(&port, "port", "p", momo.DefaultPort, "The port for momo to listen on")
	cmd.Flags().StringVar(&opts.Path, "path", "", "The base URL path for momo")

	return cmd
}

func NewUnpack() *cobra.Command {
	var (
		cmd = &cobra.Command{Use: "unpack"}
	)

	cmd.AddCommand(
		NewUnpackManifest(),
		NewUnpackMetadata(),
	)

	return cmd
}

func NewUnpackManifest() *cobra.Command {
	var (
		cmd = &cobra.Command{
			Use:  "manifest (.apk)",
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				apk := android.NewAPKDecoder(args[0])
				defer apk.Close()

				manifest, err := apk.Manifest(cmd.Context())
				if err != nil {
					return err
				}

				enc := xml.NewEncoder(cmd.OutOrStdout())
				defer enc.Close()
				enc.Indent("", "  ")

				return enc.Encode(manifest)
			},
		}
	)

	return cmd
}

func NewUnpackMetadata() *cobra.Command {
	var (
		cmd = &cobra.Command{
			Use:  "metadata (.apk)",
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				apk := android.NewAPKDecoder(args[0])
				defer apk.Close()

				metadata, err := apk.Metadata(cmd.Context())
				if err != nil {
					return err
				}

				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")

				return enc.Encode(metadata)
			},
		}
	)

	return cmd
}
