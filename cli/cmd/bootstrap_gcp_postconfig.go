package cmd

import (
	"log"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"
)

type BootstrapGcpPostconfigCmd struct {
	cmd *cobra.Command

	Opts *BootstrapGcpPostconfigOpts
}

type BootstrapGcpPostconfigOpts struct {
	*GlobalOptions
	InstallConfigPath string
	PrivateKeyPath    string
}

func (c *BootstrapGcpPostconfigCmd) RunE(_ *cobra.Command, args []string) error {
	log.Printf("running post-configuration steps...")

	return nil
}

func AddBootstrapGcpPostconfigCmd(bootstrapGcp *cobra.Command, opts *GlobalOptions) {
	postconfig := BootstrapGcpPostconfigCmd{
		cmd: &cobra.Command{
			Use:   "postconfig",
			Short: "Run post-configuration steps for GCP bootstrapping",
			Long: io.Long(`After bootstrapping GCP infrastructure, this command runs additional configuration steps
							to finalize the setup for the Codesphere cluster on GCP:

							* Install Google Cloud Controller Manager for ingress management.`),
		},
		Opts: &BootstrapGcpPostconfigOpts{
			GlobalOptions: opts,
		},
	}

	flags := postconfig.cmd.Flags()
	flags.StringVar(&postconfig.Opts.InstallConfigPath, "install-config-path", "config.yaml", "Path to the installation configuration file")
	flags.StringVar(&postconfig.Opts.PrivateKeyPath, "private-key-path", "", "Path to the GCP service account private key file (optional)")

	bootstrapGcp.AddCommand(postconfig.cmd)
	postconfig.cmd.RunE = postconfig.RunE
}
