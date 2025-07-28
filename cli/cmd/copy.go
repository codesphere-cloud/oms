/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/codesphere-cloud/oms/pkg/ssh"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// CopyCmd represents the copy command
type CopyCmd struct {
	cmd             *cobra.Command
	source          *string
	destination     *string
	destinationPath *string
	identity        *string
}

func (c *CopyCmd) RunE(_ *cobra.Command, args []string) (err error) {
	//Command execution goes here

	fmt.Printf("running %s\n", c.cmd.Use)

	err = assertMandatoryParameter(c.source, "--src", err)
	err = assertMandatoryParameter(c.destination, "--dest", err)
	err = assertMandatoryParameter(c.destinationPath, "--path", err)
	err = assertMandatoryParameter(c.identity, "--identity", err)

	if err != nil {
		return fmt.Errorf("copy failed: %w", err)
	}

	ssh.CopyRecursively(*c.source, *c.destination, *c.destinationPath, *c.identity)

	return nil
}

func AddCopyCmd(rootCmd *cobra.Command) {
	copy := CopyCmd{
		cmd: &cobra.Command{
			Use:   "cp",
			Short: "Copies files to destination hosts",
			Long: `Copies files from the local machine to target hosts.

Requires an established key exchange with the target host, i.e. you need to be able to run
ssh <host>
without entering a password. Exchanged key can be specified using the -i flag.
Example command:
` + os.Args[0] + ` cp -i ~/.ssh/id_ed25519 -s src-dir -d root@<host> -p <destination_path>`,
		},
	}
	copy.source = copy.cmd.Flags().StringP("src", "s", "", "source path")
	copy.destination = copy.cmd.Flags().StringP("dest", "d", "", "destination host in the form user@host")
	copy.destinationPath = copy.cmd.Flags().StringP("path", "p", "", "path at destination host")
	copy.identity = copy.cmd.Flags().StringP("identity", "i", "", "identity file to use for the SSH connection")
	rootCmd.AddCommand(copy.cmd)
	copy.cmd.RunE = copy.RunE
}

func assertMandatoryParameter(value *string, parameter string, errIn error) (err error) {
	if value == nil || *value == "" {
		msg := "parameter expected but not found: " + parameter
		if errIn == nil {
			return errors.New(msg)
		}
		return fmt.Errorf("%s, %s", errIn.Error(), msg)
	}
	return err
}
