// Command panelctl is the standalone build of "vkai panel": it reads and
// changes how the VKAI Panel is reached - its own port, its security entrance,
// the IP allow list and the pinned domain. It is the escape hatch for an
// operator who changed the entrance and then closed the browser tab.
//
//	vkai panel info
//	vkai panel port 8888
//	vkai panel entrance random
//	vkai panel allow-ip 203.0.113.7,10.0.0.0/8
//	vkai panel domain panel.example.com
//
// The same commands are available from the main "vkai" binary; this build
// exists so the panel can be recovered without the full CLI installed. Every
// change is written to the panel access state file
// (/vkai-panel/etc/panel_access.json by default) and takes effect after
// "systemctl restart vkai-api".
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/hitechcloud-vietnam/vkai-panel/internal/cli"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Loi: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	// The displayed name is the brand command, not the binary path: every
	// example an operator copies out of the help text has to work as typed on
	// the main CLI too.
	root := &cobra.Command{
		Use:           "vkai",
		Short:         "Quan tri cong truy cap va loi vao an toan cua VKAI Panel",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(cli.NewPanelCmd())
	return root
}
