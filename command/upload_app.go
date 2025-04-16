package command

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/frantjc/momo"
	"github.com/frantjc/momo/android"
	momov1alpha1 "github.com/frantjc/momo/api/v1alpha1"
	"github.com/frantjc/momo/internal/momoutil"
	"github.com/frantjc/momo/ios"
	xslice "github.com/frantjc/x/slice"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewUpload returns the command which acts as
// the entrypoint for `appa upload app`.
func NewUploadApp() *cobra.Command {
	return newUploadApp("app")
}

// NewKubectlUploadApp returns the command which acts as
// the entrypoint for `kubectl-upload_app`.
func NewKubectlUploadApp() *cobra.Command {
	return newUploadApp("kubectl-upload_app")
}

// newUploadApp returns the command which acts as the
// entrypoint for `kubectl upload-app` and `appa upload app`.
func newUploadApp(name string) *cobra.Command {
	var (
		addr         string
		bucketName   string
		isIPA, isAPK bool
		bucketLabels map[string]string
		cfgFlags     = genericclioptions.NewConfigFlags(true)
		cmd          = &cobra.Command{
			Use: name,
			RunE: func(cmd *cobra.Command, args []string) error {
				var (
					ctx     = cmd.Context()
					appName = args[0]
					file    = args[1]
					cliCfg  = cfgFlags.ToRawKubeConfigLoader()
					cli     = new(momo.Client)
					kubeCli client.Client
				)

				namespace, ok, err := cliCfg.Namespace()
				if err != nil {
					return err
				} else if !ok || namespace == "" {
					namespace = "default"
				}

				if addr != "" {
					var err error
					if cli.BaseURL, err = url.Parse(addr); err != nil {
						return err
					}
				}

				restCfg, err := cliCfg.ClientConfig()
				if err == nil {
					cli.HTTPClient = http.DefaultClient
					cli.HTTPClient.Transport = &kubeAuthTransport{RestConfig: restCfg}

					scheme := runtime.NewScheme()

					if err := momov1alpha1.AddToScheme(scheme); err != nil {
						return err
					}

					kubeCli, err = client.New(restCfg, client.Options{Scheme: scheme})
					if err != nil {
						return err
					}

					if bucketName == "" {
						buckets := &momov1alpha1.BucketList{}

						if err = kubeCli.List(ctx, buckets, &client.ListOptions{
							Namespace:     namespace,
							LabelSelector: labels.Set(bucketLabels).AsSelector(),
						}); err != nil {
							return err
						}

						if lenBuckets := len(buckets.Items); lenBuckets == 0 {
							return fmt.Errorf("unable to determine bucket name: no buckets found\n" +
								"use --bucket to specify a bucket")
						} else if lenBuckets != 1 {
							return fmt.Errorf("unable to determine bucket name: %d buckets found: %s\n"+
								"use --bucket or --bucket-label to specify a bucket",
								lenBuckets,
								strings.Join(
									xslice.Map(buckets.Items, func(bucket momov1alpha1.Bucket, _ int) string {
										return bucket.Name
									}),
									", ",
								),
							)
						}

						bucketName = buckets.Items[0].Name
					}
				}

				if bucketName == "" {
					return fmt.Errorf("--bucket is required")
				}

				if err := cli.UploadApp(ctx, file, namespace, bucketName, appName); err != nil {
					if !cmd.Flag("addr").Changed && kubeCli != nil {
						mediaType := ios.ContentTypeIPA
						if isAPK {
							mediaType = android.ContentTypeAPK
						} else if !isIPA {
							ext := filepath.Ext(file)
							switch ext {
							case momo.ExtAPK:
								mediaType = android.ContentTypeAPK
							case momo.ExtIPA:
								mediaType = ios.ContentTypeIPA
							default:
								return fmt.Errorf("unable to determine if %s is an .apk or .ipa", file)
							}
						}

						f, err := os.Open(file)
						if err != nil {
							return err
						}
						defer func() {
							_ = f.Close()
						}()

						if err := momoutil.UploadApp(ctx, kubeCli, namespace, appName, bucketName, mediaType, f); err != nil {
							return err
						}
					}

					return err
				}

				return nil
			},
		}
	)

	cfgFlags.AddFlags(cmd.Flags())
	cmd.Flags().StringVarP(&addr, "addr", "a", "", "")
	cmd.Flags().StringVarP(&bucketName, "bucket", "b", "", "")
	cmd.Flags().StringToStringVarP(&bucketLabels, "bucket-label", "l", nil, "")
	cmd.Flags().BoolVar(&isAPK, "apk", false, "")
	cmd.Flags().BoolVar(&isIPA, "ipa", false, "")

	return cmd
}
