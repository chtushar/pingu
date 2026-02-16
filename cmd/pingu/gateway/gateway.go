package gateway

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "gateway",
	Short: "Start the gateway server",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("gateway")
	},
}
