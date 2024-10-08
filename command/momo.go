package command

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	unixtable "github.com/frantjc/go-encoding-unixtable"
	"github.com/frantjc/go-ingress"
	"github.com/frantjc/momo"
	"github.com/frantjc/momo/internal/momohttp"
	"github.com/frantjc/momo/internal/momopubsub"
	"github.com/frantjc/momo/internal/momoregexp"
	"github.com/frantjc/momo/internal/momosql"
	xslice "github.com/frantjc/x/slice"
	"github.com/spf13/cobra"
	"gocloud.dev/blob"
	"gocloud.dev/postgres"
	"gocloud.dev/pubsub"
)

func retry(fn func() error, retries int) error {
	for i := 0; true; i++ {
		if err := fn(); err == nil {
			break
		} else if i >= retries {
			return err
		}

		time.Sleep(time.Second * time.Duration(i) * 2)
	}

	return nil
}

// NewMomo returns the root command for
// momo which acts as its CLI entrypoint.
func NewMomo() *cobra.Command {
	var (
		urlstr    string
		verbosity int
		cmd       = &cobra.Command{
			Use:           "momo",
			Version:       momo.SemVer(),
			SilenceErrors: true,
			SilenceUsage:  true,
			PersistentPreRun: func(cmd *cobra.Command, _ []string) {
				if verbose := os.Getenv("MOMO_VERBOSE"); verbose != "" && xslice.Some([]string{"1", "y", "yes", "true", "t"}, func(s string, _ int) bool {
					return strings.EqualFold(s, verbose)
				}) {
					verbosity = 2
				}

				cmd.SetContext(
					momo.WithLogger(
						cmd.Context(), momo.NewLogger().V(2-verbosity),
					),
				)
			},
		}
	)

	cmd.SetVersionTemplate("{{ .Name }}{{ .Version }} " + runtime.Version() + "\n")
	cmd.PersistentFlags().CountVarP(&verbosity, "verbose", "V", "verbosity for momo")
	cmd.PersistentFlags().StringVar(&urlstr, "url", "", "base URL for momo")

	cmd.AddCommand(newPing(), newGet(), newUpload(), newSrv())

	return cmd
}

func newSrv() *cobra.Command {
	var (
		address      string
		dburlstr     string
		pubsuburlstr string
		bloburlstr   string
		cmd          = &cobra.Command{
			Use:           "srv [flags] [subcommand]",
			Version:       momo.SemVer(),
			SilenceErrors: true,
			SilenceUsage:  true,
			RunE: func(cmd *cobra.Command, args []string) error {
				var (
					ctx = cmd.Context()
					log = momo.LoggerFrom(ctx)
				)

				bloburl, err := url.Parse(bloburlstr)
				if err != nil {
					return err
				}

				if bloburl.Scheme == "file" {
					if err = os.MkdirAll(bloburl.Path, 0o644); err != nil {
						return err
					}
				}

				log.Info("opening bucket " + bloburl.String())
				bucket, err := blob.OpenBucket(ctx, bloburl.String())
				if err != nil {
					return err
				}
				defer bucket.Close()

				var db *sql.DB
				if dburlstr == "" {
					dburlstr = os.Getenv("MOMO_DB_URL")
				}

				log.Info("opening postgres " + dburlstr)
				if err = retry(func() error {
					db, err = postgres.Open(ctx, dburlstr)
					return err
				}, 9); err != nil {
					return err
				}
				defer db.Close()

				log.Info("running migrations against postgres " + dburlstr)
				if err = retry(func() error {
					return momosql.Migrate(ctx, db)
				}, 9); err != nil {
					return err
				}

				log.Info("opening topic " + pubsuburlstr)
				topic, err := pubsub.OpenTopic(ctx, pubsuburlstr)
				if err != nil {
					return err
				}
				defer topic.Shutdown(ctx) //nolint:errcheck

				log.Info("opening subscription " + pubsuburlstr)
				subscription, err := pubsub.OpenSubscription(ctx, pubsuburlstr)
				if err != nil {
					return err
				}
				defer subscription.Shutdown(ctx) //nolint:errcheck

				var (
					base = new(url.URL)
					errC = make(chan error)
					ing  = &ingress.Ingress{}
				)
				if urlstr := cmd.Flag("url").Value.String(); urlstr != "" {
					if base, err = url.Parse(urlstr); err != nil {
						return err
					}
				}

				if len(args) > 0 {
					var exec *exec.Cmd
					ing.DefaultBackend, exec, err = momohttp.NewExecHandlerWithPortFromEnv(ctx, args[0], args[1:]...)
					if err != nil {
						return err
					}

					// A rough algorithm for making the working directory of
					// the exec the directory of the entrypoint in the case
					// of the args being something like `node /app/server.js`.
					for _, entrypoint := range args[1:] {
						if fi, err := os.Stat(entrypoint); err == nil {
							if fi.IsDir() {
								exec.Dir = filepath.Clean(entrypoint)
							} else {
								exec.Dir = filepath.Dir(entrypoint)
							}
							break
						}
					}

					go func() {
						log.Info("running exec fallback handler")
						errC <- exec.Run()
					}()
				}

				ing.Paths = []ingress.Path{
					ingress.ExactPath("/healthz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						fmt.Fprint(w, "ok")
					})),
					ingress.ExactPath("/readyz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						fmt.Fprint(w, "ok")
					})),
					ingress.PrefixPath("/api", momohttp.NewAPIHandler(bucket, db, topic, ing.DefaultBackend)),
					ingress.PrefixPath("/apps", momohttp.NewAppsHandler(bucket, db, base, ing.DefaultBackend)),
				}

				var (
					srv = &http.Server{
						ReadHeaderTimeout: time.Second * 5,
						BaseContext: func(_ net.Listener) context.Context {
							return ctx
						},
						Handler: ing,
					}
				)
				defer srv.Close()

				lis, err := net.Listen("tcp", address)
				if err != nil {
					return err
				}
				defer lis.Close()

				go func() {
					log.Info("listening on " + address)
					errC <- srv.Serve(lis)
				}()

				go func() {
					log.Info("receiving messages on " + pubsuburlstr)
					errC <- momopubsub.Receive(ctx, bucket, db, subscription)
				}()

				select {
				case <-ctx.Done():
					return ctx.Err()
				case err := <-errC:
					return err
				}
			},
		}
	)

	cmd.Flags().StringVar(&address, "addr", ":8080", "listen address for momo")
	cmd.Flags().StringVar(&dburlstr, "db", "", "database URL for momo")
	cmd.Flags().StringVar(&pubsuburlstr, "pubsub", "mem://", "pubsub URL for momo")
	cmd.Flags().StringVar(&bloburlstr, "blob", "mem://", "blob URL for momo")

	return cmd
}

func newPing() *cobra.Command {
	var (
		cmd = &cobra.Command{
			Use:           "ping [flags]",
			Version:       momo.SemVer(),
			SilenceErrors: true,
			SilenceUsage:  true,
			RunE: func(cmd *cobra.Command, _ []string) error {
				var (
					ctx = cmd.Context()
					cli = new(momo.Client)
				)

				if urlstr := cmd.Flag("url").Value.String(); urlstr != "" {
					var err error
					if cli.Base, err = url.Parse(urlstr); err != nil {
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

func newGet() *cobra.Command {
	var (
		cmd = &cobra.Command{
			Use:           "get",
			Version:       momo.SemVer(),
			SilenceErrors: true,
			SilenceUsage:  true,
		}
	)

	cmd.AddCommand(newGetApp())
	cmd.AddCommand(newGetApps())

	return cmd
}

func newGetApps() *cobra.Command {
	var (
		cmd = &cobra.Command{
			Use:           "apps [flags]",
			Version:       momo.SemVer(),
			SilenceErrors: true,
			SilenceUsage:  true,
			RunE: func(cmd *cobra.Command, _ []string) error {
				var (
					ctx = cmd.Context()
					cli = new(momo.Client)
				)

				if urlstr := cmd.Flag("url").Value.String(); urlstr != "" {
					var err error
					if cli.Base, err = url.Parse(urlstr); err != nil {
						return err
					}
				}

				apps, err := cli.GetApps(ctx)
				if err != nil {
					return err
				}

				if len(apps) == 0 {
					return fmt.Errorf("no apps found")
				}

				return unixtable.NewEncoder(cmd.OutOrStdout()).Encode(apps)
			},
		}
	)

	return cmd
}

func newGetApp() *cobra.Command {
	var (
		cmd = &cobra.Command{
			Use:           "app [flags] (name [version] | ID)",
			Version:       momo.SemVer(),
			Args:          cobra.RangeArgs(1, 2),
			SilenceErrors: true,
			SilenceUsage:  true,
			RunE: func(cmd *cobra.Command, args []string) error {
				var (
					ctx = cmd.Context()
					app = &momo.App{}
					cli = new(momo.Client)
				)

				switch len(args) {
				case 1:
					switch {
					case momoregexp.IsUUID(args[0]):
						app.ID = args[0]
					case momoregexp.IsAppName(args[0]):
						app.Name = args[0]
					default:
						return fmt.Errorf("invalid argument %s", args[0])
					}
				case 2:
					if momoregexp.IsAppName(args[0]) && momoregexp.IsAppVersion(args[1]) {
						app.Name = args[0]
						app.Version = args[1]
					} else {
						return fmt.Errorf("invalid arguments %s %s", args[0], args[1])
					}
				}

				if urlstr := cmd.Flag("url").Value.String(); urlstr != "" {
					var err error
					if cli.Base, err = url.Parse(urlstr); err != nil {
						return err
					}
				}

				if err := cli.GetApp(ctx, app); err != nil {
					return err
				}

				return unixtable.NewEncoder(cmd.OutOrStdout()).Encode(app)
			},
		}
	)

	return cmd
}

func newUpload() *cobra.Command {
	var (
		cmd = &cobra.Command{
			Use:           "upload",
			Version:       momo.SemVer(),
			SilenceErrors: true,
			SilenceUsage:  true,
		}
	)

	cmd.AddCommand(newUploadApp())

	return cmd
}

func newUploadApp() *cobra.Command {
	var (
		cmd = &cobra.Command{
			Use:           "app [flags] (name) [version] (.ipa|.apk ...)",
			Version:       momo.SemVer(),
			Args:          cobra.RangeArgs(2, 4),
			SilenceErrors: true,
			SilenceUsage:  true,
			RunE: func(cmd *cobra.Command, args []string) error {
				var (
					ctx = cmd.Context()
					app = &momo.App{
						Name: args[0],
					}
					cli = new(momo.Client)
				)

				if !momoregexp.IsAppName(app.Name) {
					return fmt.Errorf("invalid app name %s", app.Name)
				}

				var (
					pr, pw     = io.Pipe()
					gz         = gzip.NewWriter(pw)
					tw         = tar.NewWriter(gz)
					filesIndex = 1
				)

				switch len(args) {
				case 3:
					if !momoregexp.IsApp(args[1]) && momoregexp.IsAppVersion(args[1]) {
						app.Version = args[1]
						filesIndex = 2
					}
				case 4:
					if momoregexp.IsAppVersion(args[1]) {
						app.Version = args[1]
					} else {
						return fmt.Errorf("invalid app version %s", args[1])
					}
					filesIndex = 2
				}

				go func() {
					if err := func() error {
						for _, arg := range args[filesIndex:] {
							if momoregexp.IsApp(arg) {
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
							} else {
								return fmt.Errorf("invalid app %s", arg)
							}
						}

						return nil
					}(); err != nil {
						_ = tw.Close()
						_ = gz.Close()
						_ = pw.CloseWithError(err)
						return
					}

					_ = tw.Close()
					_ = gz.Close()
					_ = pw.Close()
				}()

				if urlstr := cmd.Flag("url").Value.String(); urlstr != "" {
					var err error
					if cli.Base, err = url.Parse(urlstr); err != nil {
						return err
					}
				}

				if err := cli.UploadApp(ctx, pr, "application/x-gzip", app); err != nil {
					return err
				}

				return unixtable.NewEncoder(cmd.OutOrStdout()).Encode(app)
			},
		}
	)

	return cmd
}
