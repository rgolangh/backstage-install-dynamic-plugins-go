package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type PluginsFile struct {
	Plugins  []Plugin `yaml:"plugins"`
	Includes []string `yaml:"includes"`
}

type Plugin struct {
	Package      string                 `yaml:"package"`
	Integrity    string                 `yaml:"integrity,omitempty"`
	Disabled     bool                   `yaml:"disabled"`
	PluginConfig map[string]interface{} `yaml:"pluginConfig,omitempty"`
}

// rootCmd represents the base command when called without any subcommands
func main() {
	dynamicPluginsRoot := flag.String("dynamic-plugins-root", "./dynamic-plugins-root", "dynamic plugins root")
	dynamicPluginsFile := flag.String("dynamic-plugins-file", "./dynamic-plugins.default.yaml", "dynamic plugins file")
	skipIntegrityCheck := flag.Bool("skip-integrity-check", false, "skip integrity check for the plugins")
	flag.Parse()

	dynamicPluginsGlobalConfigFile := path.Join(*dynamicPluginsRoot, "app-config.dynamic-plugins.yaml")
	globalConfig := make(map[string]interface{})
	globalConfig["dynamicPlugins"] = map[string]interface{}{"rootDirectory": "dynamic-plugins-root"}

	fmt.Printf("dynamic plugin root %s\n", *dynamicPluginsRoot)
	fmt.Printf("dynamic plugin file %s\n", *dynamicPluginsFile)
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("failed to get the current working dir %s\n", err)
		return
	}
	bytes, err := os.ReadFile(*dynamicPluginsFile)

	if err != nil {
		panic(fmt.Errorf("Faild to open the plugins file %w", err))
	}

	allPlugins := make(map[string]Plugin)
	plugins := PluginsFile{}
	err = yaml.Unmarshal(bytes, &plugins)
	if err != nil {
		panic(fmt.Errorf("failed to read the plugins file %w", err))
	}

	for _, include := range plugins.Includes {
		includePluginFile := PluginsFile{}
		bytes, err := os.ReadFile(include)
		if err != nil {
			panic(fmt.Sprintf("failed reading include file %s due to %s", include, err))
		}
		err = yaml.Unmarshal(bytes, &includePluginFile)
		if err != nil {
			panic(fmt.Sprintf("failed to read the plugins file %w", err))
		}

		for _, p := range includePluginFile.Plugins {
			allPlugins[p.Package] = p
		}
	}

	for _, p := range plugins.Plugins {
		allPlugins[p.Package] = p
	}

	maxEntrySize, ok := os.LookupEnv("MAX_ENTRY_SIZE")
	if !ok {
		maxEntrySize = "20000000"
	}
	maxEntrySizeInt, err := strconv.Atoi(maxEntrySize)
	if err != nil {
		fmt.Printf("%s\n", err)
		return
	}
	fmt.Printf("num of plugins %d\n", len(allPlugins))
	wg := sync.WaitGroup{}
	for _, p := range allPlugins {
		plugin := p
		if plugin.Disabled {
			fmt.Printf("Plugin %s is disabled, skipping...\n", plugin.Package)
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			fmt.Printf("unpacking plugin -> %s\n", plugin.Package)

			isLocal := false
			myPackage := plugin.Package
			if plugin.Package[0:2] == "./" {
				// local package, must run integrity check
				myPackage = path.Join(cwd, plugin.Package[2:])
				isLocal = true
			}

			cmd := exec.Command("npm", "pack", myPackage)
			cmd.Dir = *dynamicPluginsRoot
			out, err := cmd.Output()
			if err != nil {
				fmt.Printf("failed invoking %s with %s\n", cmd, err)
				return
			}
			archive := path.Join(*dynamicPluginsRoot, strings.TrimSuffix(string(out), "\n"))
			if isLocal || !*skipIntegrityCheck {
				fmt.Printf("performing integrity check on package %s: ", myPackage)
				err := integrityCheck(plugin, archive)
				if err != nil {
					fmt.Printf("Failed\n")
					panic(fmt.Sprintf("Failed integrity check %e\n", err))
				}
				fmt.Printf("OK\n")
			}

			archiveDir := strings.TrimSuffix(archive, ".tgz")
			fmt.Printf("Removing previous plugin directory\n")
			err = os.Remove(archiveDir)
			if err != nil {
				fmt.Printf("Removing the directory %s failed, try to continue\n", archiveDir)
			}
			os.MkdirAll(archiveDir, fs.ModePerm)
			fmt.Printf("Extracting package archive\n")
			f, err := os.Open(archive)
			if err != nil {
				fmt.Fprint(os.Stderr, err)
				return
			}
			defer f.Close()
			gz, err := gzip.NewReader(bufio.NewReader(f))
			if err != nil {
				panic(fmt.Sprintf("failed to open gzip archive %s\n ", err))
			}

			tarReader := tar.NewReader(gz)
			for {
				h, err := tarReader.Next()
				if err == io.EOF {
					break // End of archive
				}
				if err != nil {
					panic(fmt.Sprintf("failed to open tar archive %s\n ", err))
				}
				switch h.Typeflag {
				case tar.TypeReg:
					if !strings.HasPrefix(h.Name, "package/") {
						panic(fmt.Sprintf("error %s doesn't start with 'package/'\n", h.Name))
					}
					if h.Size > int64(maxEntrySizeInt) {
						panic(fmt.Sprintf("error Zip bomb detected on %s\n", h.Name))
					}

					// omit package/ from the path
					h.Name = strings.TrimPrefix(h.Name, "package/")

					// create the dir. ok if alrady exist
					f := path.Join(archiveDir, h.Name)
					err = os.MkdirAll(path.Dir(f), fs.ModePerm)
					if err != nil {
						fmt.Printf("directory already exist, continue %s\n", err)
					}
					// get the content, and write
					b, err := io.ReadAll(tarReader)
					if err != nil {
						log.Fatal(err)
					}
					//write the file, presserve the perms from the archive
					err = os.WriteFile(f, b, h.FileInfo().Mode().Perm())
					if err != nil {
						panic(fmt.Sprintf("failed writing file %s due to %s", f, err))
					}

				case tar.TypeDir:
					// no op, it's a directory

				case tar.TypeLink:
					if !strings.HasPrefix("package/", h.Name) {
						fmt.Printf("link doesn't start with package, exiting\n")
					}
					//                 realpath, err := os.Readlink(path.Join(archiveDir, h.Name))
					if err != nil {
						fmt.Printf("reading link path failed %s\n", err)
						return
					}
					b, err := io.ReadAll(tarReader)
					if err != nil {
						fmt.Printf("failed to read file %s\n", err)
						return
					}

					err = os.WriteFile(path.Join(archiveDir, h.Name), b, h.FileInfo().Mode())
					if err != nil {
						fmt.Printf("failed to read file %s\n", err)
						return
					}
				default:
					fmt.Printf("got an irregular file %s to handle in the package %s\n", h.Typeflag, h.Name)
				}

			}
			mergeMaps(plugin.PluginConfig, globalConfig)
		}()
	}

	wg.Wait()
	fmt.Printf("done unpacking %d plugins\n", len(allPlugins))

	out, err := yaml.Marshal(globalConfig)
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(dynamicPluginsGlobalConfigFile, out, fs.ModePerm)
	if err != nil {
		panic(fmt.Sprintf("failed to write the merge plugins config file due to %s", err))
	}
}

func integrityCheck(plugin Plugin, archive string) error {
	if plugin.Integrity == "" {
		return fmt.Errorf("Plugin %s integrity field is missing or empty", plugin.Package)
	}

	integrity := strings.Split(plugin.Integrity, "-")
	if len(integrity) != 2 {
		return fmt.Errorf("Plugin %s integrity value is missing or can't be parsed (not in the form of sha{512,348,256}-xxxxxx)", plugin.Package)
	}
	b, err := os.ReadFile(archive)
	if err != nil {
		return err
	}
	var sum string
	algo := integrity[0]
	switch algo {
	case "sha512":
		s := sha512.Sum512(b)
		sum = base64.StdEncoding.EncodeToString(s[:])
	case "sha384":
		s := sha512.Sum384(b)
		sum = base64.StdEncoding.EncodeToString(s[:])
	case "sha256":
		s := sha512.Sum512_256(b)
		sum = base64.StdEncoding.EncodeToString(s[:])
	default:
		return fmt.Errorf("unsupported hash algorythm %q", algo)
	}

	if sum != integrity[1] {
		return fmt.Errorf("failed integrity check wanted %q got %q algorythm %s", integrity[1], sum, algo)
	}
	return nil
}

func mergeMaps(s map[string]interface{}, d map[string]interface{}) {
	for k, v := range s {
		srcMap, isMap := v.(map[string]interface{})

		// if key both in src and dest
		//   if the value is not a map, insert the value of source into dst
		//   if the value is a map, recurse
		// if the key is only on dst, skip
		// if the key is only on src , insert into dst

		if _, exists := d[k]; exists {
			if isMap {
				dstMap, _ := d[k].(map[string]interface{})
				mergeMaps(srcMap, dstMap)
				continue
			}
		}
		d[k] = v
	}
}
