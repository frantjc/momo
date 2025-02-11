package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
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

	"github.com/frantjc/go-ingress"
	"github.com/frantjc/momo"
	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/internal/controller"
	"github.com/frantjc/momo/internal/momoutil"
	xos "github.com/frantjc/x/os"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"gocloud.dev/blob"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	//+kubebuilder:scaffold:imports
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
	//+kubebuilder:scaffold:scheme
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

				if err = (&controller.UploadReconciler{}).SetupWithManager(mgr); err != nil {
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
		addr     string
		cmd = &cobra.Command{
			Use:           "srv",
			SilenceErrors: true,
			SilenceUsage:  true,
			RunE: func(cmd *cobra.Command, args []string) error {
				l, err := net.Listen("tcp", addr)
				if err != nil {
					return err
				}
				defer l.Close()

				zHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write([]byte("ok\n"))
				})
				paths := []ingress.Path{
					ingress.ExactPath("/readyz", zHandler),
					ingress.ExactPath("/livez", zHandler),
					ingress.ExactPath("/healthz", zHandler),
					ingress.PrefixPath("/api/v1", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var (
							ctx    = r.Context()
							pretty = momoutil.IsPretty(r)
						)

						spl := strings.Split(strings.ToLower(r.URL.Path), "/")

						if len(spl) !=  6 || spl[4] != "uploads" {
							http.NotFound(w, r)
							return
						}

						var (
							namespace = spl[2]
							bucketName = spl[3]
							appName = spl[5]
						)

						mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
						if err != nil {
							_ = momoutil.RespondErrorJSON(w, err, pretty)
							return
						}

						var (
							boundary      = params["boundary"]
							isMultipart = strings.EqualFold(mediaType, momoutil.ContentTypeMultiPart)
							isTar       = strings.EqualFold(mediaType, momoutil.ContentTypeTar)
							isApk       = strings.EqualFold(mediaType, momoutil.ContentTypeAPK)
							isIpa       = strings.EqualFold(mediaType, momoutil.ContentTypeIPA)
							isGzip      = strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip")
						)

						if isMultipart || isTar || isApk || isIpa {
							if isMultipart && boundary == "" {
								_ = momoutil.RespondErrorJSON(w,
									momoutil.NewHTTPStatusCodeError(
										fmt.Errorf("no boundary"),
										http.StatusBadRequest,
									),
									pretty,
								)
								return
							}

							cfg, err := momoutil.GetConfigForRequest(r)
							if err != nil {
								_ = momoutil.RespondErrorJSON(w, err, pretty)
								return
							}

							cli, err := client.New(cfg, client.Options{
								Scheme: scheme,
							})
							if err != nil {
								_ = momoutil.RespondErrorJSON(w, err, pretty)
								return
							}

							var (
								bucket = &momov1alpha1.Bucket{}
							)

							if err = cli.Get(ctx, client.ObjectKey{
								Name: bucketName,
								Namespace: namespace,
							}, bucket); err != nil {
								_ = momoutil.RespondErrorJSON(w, err, pretty)
								return
							}

							b, err := momoutil.OpenBucket(ctx, cli, bucket)
							if err != nil {
								_ = momoutil.RespondErrorJSON(w, err, pretty)
								return
							}

							var (
								key    = ""
								upload = &momov1alpha1.Upload{
									ObjectMeta: metav1.ObjectMeta{
										Name: appName,
										Namespace: namespace,
									},
									Spec: momov1alpha1.UploadSpec{
										SpecBucketKeyRef: momov1alpha1.SpecBucketKeyRef{
											Key: key,
											Bucket: corev1.LocalObjectReference{
												Name: bucket.Name,
											},
										},
									},
								}
							)

							if err = cli.Create(ctx, upload); err != nil {
								_ = momoutil.RespondErrorJSON(w, err, pretty)
								return
							}

							bw, err := b.NewWriter(ctx, key, &blob.WriterOptions{
								ContentType:     momoutil.ContentTypeTar,
								ContentEncoding: "gzip",
							})
							if err != nil {
								_ = momoutil.RespondErrorJSON(w, err, pretty)
								return
							}
							if !isGzip {
								defer bw.Close()
							}
				
							var (
								wc   io.WriteCloser = bw
								body io.Reader      = r.Body
							)
							if !isGzip {
								wc = gzip.NewWriter(bw)
							}
							defer wc.Close()
				
							switch {
							case isMultipart:
								body = momoutil.MultipartToTar(multipart.NewReader(r.Body, boundary), nil)
							case isApk:
								body = momoutil.FileToTar(r.Body, "app.apk", nil)
							case isIpa:
								body = momoutil.FileToTar(r.Body, "app.ipa", nil)
							}
				
							if _, err = io.Copy(wc, body); err != nil {
								_ = momoutil.RespondErrorJSON(w, err, pretty)
								return
							}
						} else {
							_ = momoutil.RespondErrorJSON(w,
								momoutil.NewHTTPStatusCodeError(
									fmt.Errorf("unsupported Content-Type %s", mediaType),
									http.StatusUnsupportedMediaType,
								),
								pretty,
							)
						}
				
						if isMultipart {
							if referer := r.Header.Get("Referer"); referer != "" {
								http.Redirect(w, r, referer, http.StatusFound)
								return
							}
						}
				
						w.WriteHeader(http.StatusCreated)
						_ = momoutil.RespondJSON(w, map[string]string{"hello": "there"}, pretty)
					})),
				}

				srv := &http.Server{
					Addr:              addr,
					ReadHeaderTimeout: time.Second * 5,
					Handler:           ingress.New(paths...),
				}

				eg, ctx := errgroup.WithContext(cmd.Context())

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

	return cmd
}

func NewPing() *cobra.Command {
	var (
		addr string
		cmd  = &cobra.Command{
			Use:           "ping [flags]",
			SilenceErrors: true,
			SilenceUsage:  true,
			RunE: func(cmd *cobra.Command, _ []string) error {
				var (
					ctx = cmd.Context()
					cli = new(momo.Client)
				)

				if addr != "" {
					var err error
					if cli.BaseURL, err = url.Parse(addr); err != nil {
						return err
					}
				}

				if err := cli.Readyz(ctx); err != nil {
					return err
				}

				if err := cli.Healthz(ctx); err != nil {
					return err
				}

				return nil
			},
		}
	)

	return cmd
}

func NewUpload() *cobra.Command {
	var (
		addr string
		cmd  = &cobra.Command{
			Use:           "upload [flags] (namespace) (bucket) (name) (.ipa|.apk|.png...)",
			Args:          cobra.MinimumNArgs(3),
			SilenceErrors: true,
			SilenceUsage:  true,
			RunE: func(cmd *cobra.Command, args []string) error {
				var (
					ctx    = cmd.Context()
					namespace = args[0]
					bucketName = args[1]
					appName   = args[2]
					cli    = new(momo.Client)
				)

				var (
					pr, pw = io.Pipe()
					tw     = tar.NewWriter(pw)
				)

				go func() {
					err := func() error {
						for _, arg := range args[2:] {
							f, err := os.Open(arg)
							if err != nil {
								return err
							}
							defer f.Close()

							fi, err := f.Stat()
							if err != nil {
								return err
							}

							hdr, err := tar.FileInfoHeader(fi, fi.Name())
							if err != nil {
								return err
							}

							if err = tw.WriteHeader(hdr); err != nil {
								return err
							}

							if _, err = io.Copy(tw, f); err != nil {
								return err
							}
						}

						return nil
					}()

					_ = tw.Close()
					_ = pw.CloseWithError(err)
				}()

				if addr != "" {
					var err error
					if cli.BaseURL, err = url.Parse(addr); err != nil {
						return err
					}
				}

				if err := cli.UploadApp(ctx, pr, namespace, bucketName, appName); err != nil {
					return err
				}

				return nil
			},
		}
	)

	return cmd
}
