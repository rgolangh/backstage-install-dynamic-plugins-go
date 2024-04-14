/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>

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
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"sync"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/spf13/viper"
)

var (
	cfgFile            string
	dynamicPluginsRoot string
	dynamicPluginsFile string
)

type PluginsFile struct {
	Plugins []Plugin `yaml:"plugins"`
}

type Plugin struct {
	Package   string `yaml:"package"`
	Integrity string `yaml:"integrity"`
	Disabled  bool   `yaml:"disabled"`
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "backstage-install-dynamic-plugins-go",
	Short: "A brief description of your application",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {

		fmt.Printf("dynamic plugin root %s\n", dynamicPluginsRoot)
		fmt.Printf("dynamic plugin file %s\n", dynamicPluginsFile)

		bytes, err := os.ReadFile(dynamicPluginsFile)

		if err != nil {
			return fmt.Errorf("Faild to open the plugins file %w", err)
		}

		plugins := PluginsFile{}
		err = yaml.Unmarshal(bytes, &plugins)
		if err != nil {
			return fmt.Errorf("failed to read the plugins file %w", err)
		}

        maxloop := 55
		fmt.Printf("num of plugins %d\n", len(plugins.Plugins))
		wg := sync.WaitGroup{}
		for i, p := range plugins.Plugins {
            if i == maxloop {
                fmt.Printf("breaking after %d %d\n",i, maxloop)
                break 
            }
            wg.Add(1)
			go func() {
				defer wg.Done()
				fmt.Printf("unpacking plugin -> %s\n", p.Package)

				myPackage := p.Package
				if p.Package[0:2] == "./" {
					// local package, must run integrity check
                    cwd, err := os.Getwd()
					if err != nil {
						fmt.Printf("failed to get the current working dir %s\n", err)
                        return
					}

					myPackage = path.Join(cwd, p.Package[2:])
                    cmd := exec.Command("npm", "pack", myPackage)
                    //cmd.Dir = dynamicPluginsRoot
                    err = cmd.Run()
                    if err != nil {
                        fmt.Printf("failed invoking %s with %s\n", cmd, err)
                        return 
                    }
				}

			}()
		}

		wg.Wait()
		fmt.Print("done unpacking plugins")
		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.backstage-install-dynamic-plugins-go.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("help", "h", false, "Help message for toggle")
	rootCmd.Flags().BoolP("skip-integrity-check", "", false, "skip integrity check for the plugins")

	rootCmd.Flags().StringVar(&dynamicPluginsRoot, "dynamic-plugins-root", "/tmp/foo", "dynamic plugins root")
	rootCmd.Flags().StringVar(&dynamicPluginsFile, "dynamic-plugins-file", "/tmp/dynamic-plugins.default.yaml", "dynamic plugins file")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".backstage-install-dynamic-plugins-go" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".backstage-install-dynamic-plugins-go")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
