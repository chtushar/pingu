package setup

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "setup",
	Short: "Set up pingu configuration",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("setup")
	},
}
