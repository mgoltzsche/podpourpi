package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	genericapiserver "k8s.io/apiserver/pkg/server"
)

const envVarPrefix = "PODPOURPI_"

func Execute(logger *logrus.Entry) error {
	rootCmd := cobra.Command{
		Use:           "podpourpi",
		Short:         "A simple (IoT) device management API and UI",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	ctx := genericapiserver.SetupSignalContext()
	rootCmd.SetFlagErrorFunc(handleFlagError)
	rootCmd.AddCommand(newServeCommand(ctx, logger))
	supportedEnvVars := map[string]struct{}{}
	err := applyEnvVars(&rootCmd, supportedEnvVars, logger)
	if err != nil {
		return err
	}
	err = failOnUnsupportedEnvVar(supportedEnvVars)
	if err != nil {
		return err
	}
	return rootCmd.Execute()
}

func handleFlagError(cmd *cobra.Command, err error) error {
	_ = cmd.Help()
	return err
}

func applyEnvVars(cmd *cobra.Command, supportedEnvVars map[string]struct{}, logger *logrus.Entry) (err error) {
	flags := cmd.Flags()
	flags.VisitAll(func(f *pflag.Flag) {
		envVarName := envVarPrefix + strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		supportedEnvVars[envVarName] = struct{}{}
		if envVarValue := os.Getenv(envVarName); envVarValue != "" {
			f.DefValue = envVarValue
			e := f.Value.Set(envVarValue)
			if e != nil && err == nil {
				err = fmt.Errorf("invalid environment variable %s value provided: %w", envVarValue, e)
			}
		}
	})
	if err != nil {
		flags.Usage()
		return err
	}
	for _, c := range cmd.Commands() {
		err := applyEnvVars(c, supportedEnvVars, logger)
		if err != nil {
			return err
		}
	}
	return nil
}

func failOnUnsupportedEnvVar(supportedEnvVars map[string]struct{}) error {
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, envVarPrefix) {
			kv := strings.SplitN(entry, "=", 2)
			if _, ok := supportedEnvVars[kv[0]]; !ok {
				return fmt.Errorf("unsupported environment variable provided: %s", kv[0])
			}
		}
	}
	return nil
}

func newContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cancel()
		<-c
		os.Exit(1) // exit immediately on 2nd signal
	}()
	return ctx
}
