/*
Copyright 2022 The Machine Controller Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type options struct {
	LogFormat     string
	MachineConfig string
}

var opts options

func main() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		logrus.Fatalf("Error executing machine-provisioner: %v", err)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:              filepath.Base(os.Args[0]),
		Short:            "Tool to provision machines",
		Long:             "Tool to provision machines on various cloud providers.",
		PersistentPreRun: runRootCmd,
		SilenceUsage:     true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	// Options
	cmd.PersistentFlags().StringVar(&opts.LogFormat, "log-format", "", "Log format to use (empty string for text, or JSON")
	cmd.PersistentFlags().StringVar(&opts.MachineConfig, "machine-config", "./machines.yaml", "Path to the YAML file for machines")

	cmd.AddCommand(newCreateCommand(cmd.PersistentFlags()))

	return cmd
}

func runRootCmd(cmd *cobra.Command, args []string) {
	err := configureLogging(opts.LogFormat)
	if err != nil {
		logrus.Warn(err)
	}
}

func configureLogging(logFormat string) error {
	logrus.SetLevel(logrus.DebugLevel)

	switch logFormat {
	case "json":
		logrus.SetFormatter(&logrus.JSONFormatter{})
	default:
		// just let the library use default on empty string.
		if logFormat != "" {
			return fmt.Errorf("unsupported logging formatter: %q", logFormat)
		}
	}
	return nil
}
