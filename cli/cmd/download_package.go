package cmd

import (
	"fmt"

	"github.com/codesphere-cloud/cs-go/pkg/io"
	"github.com/spf13/cobra"

	"github.com/codesphere-cloud/oms/internal/portal"
	"github.com/codesphere-cloud/oms/internal/util"
)

// DownloadPackageCmd represents the package command
type DownloadPackageCmd struct {
	cmd        *cobra.Command
	Opts       DownloadPackageOpts
	FileWriter util.FileWriter
}

type DownloadPackageOpts struct {
	GlobalOptions
	Version  string
	Hash     string
	Filename string
}

func (c *DownloadPackageCmd) RunE(_ *cobra.Command, args []string) error {
	fmt.Printf("Downloading package %s\n", c.Opts.Version)

	p := portal.NewPortalClient()
	build, err := p.GetBuild(portal.CodesphereProduct, c.Opts.Version, c.Opts.Hash)
	if err != nil {
		return fmt.Errorf("failed to get codesphere package: %w", err)
	}

	err = c.DownloadBuild(p, build, c.Opts.Filename)
	if err != nil {
		return fmt.Errorf("failed to download codesphere package: %w", err)
	}

	return nil
}

func AddDownloadPackageCmd(download *cobra.Command, opts GlobalOptions) {
	pkg := DownloadPackageCmd{
		cmd: &cobra.Command{
			Use:   "package",
			Short: "Download a codesphere package",
			Long: io.Long(`Download a specific version of a Codesphere package
				To list available packages, run oms list packages.`),
			Example: io.FormatExampleCommands("download package", []io.Example{
				{Cmd: "--version 1.55.0", Desc: "Download Codesphere version 1.55.0"},
				{Cmd: "--version 1.55.0 --file installer-lite.tar.gz", Desc: "Download lite package of Codesphere version 1.55.0"},
			}),
		},
		FileWriter: util.NewFilesystemWriter(),
	}
	pkg.cmd.Flags().StringVarP(&pkg.Opts.Version, "version", "V", "", "Codesphere version to download")
	pkg.cmd.Flags().StringVarP(&pkg.Opts.Hash, "hash", "H", "", "Codesphere version to download")
	pkg.cmd.Flags().StringVarP(&pkg.Opts.Filename, "file", "f", "installer.tar.gz", "Specify artifact to download")
	download.AddCommand(pkg.cmd)

	pkg.cmd.RunE = pkg.RunE
}

func (c *DownloadPackageCmd) DownloadBuild(p portal.Portal, build portal.Build, filename string) error {
	download, err := build.GetBuildForDownload(filename)
	if err != nil {
		return fmt.Errorf("failed to find artifact in package: %w", err)
	}
	fullFilename := build.Version + "-" + filename
	out, err := c.FileWriter.Create(fullFilename)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", fullFilename, err)
	}
	defer func() { _ = out.Close() }()

	err = p.DownloadBuildArtifact("codesphere", download, out)
	if err != nil {
		return fmt.Errorf("failed to download build: %w", err)
	}
	return nil
}
