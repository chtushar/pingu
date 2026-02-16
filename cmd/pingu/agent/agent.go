package agent

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "agent",
	Short: "Start the agent",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("agent")
	},
}
