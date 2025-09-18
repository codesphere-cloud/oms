package cmd

import (
	"fmt"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/spf13/cobra"
)

type RevokeCmd struct {
	cmd  *cobra.Command
	Opts RevokeOpts
}

type RevokeOpts struct {
	GlobalOptions
	Key string
}

func (c *RevokeCmd) RunE(_ *cobra.Command, args []string) error {
	p := portal.NewPortalClient()
	return c.Revoke(p)
}

func (c *RevokeCmd) Revoke(p portal.Portal) error {
	err := p.RevokeAPIKey(c.Opts.Key)
	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	return nil
}

func AddRevokeCmd(list *cobra.Command, opts GlobalOptions) {
	c := RevokeCmd{
		cmd: &cobra.Command{
			Use:   "revoke",
			Short: "Revoke an API key",
			Long:  io.Long(`Revoke an OMS portal API key.`),
		},
		Opts: RevokeOpts{GlobalOptions: opts},
	}
	c.cmd.Flags().StringVarP(&c.Opts.Key, "key", "k", "", "API key to revoke")

	c.cmd.RunE = c.RunE

	list.AddCommand(c.cmd)
}
