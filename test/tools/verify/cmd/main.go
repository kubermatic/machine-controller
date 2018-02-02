package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

func main() {
	var manifestPath string
	var parameters string

	flag.StringVar(&manifestPath, "input", "", "a path to the machine's manifest.")
	flag.StringVar(&parameters, "parameters", "", "a list of comma-delimited key value pairs i.e key=value,key1=value2")
	flag.Parse()

	if len(manifestPath) == 0 || len(parameters) == 0 {
		fmt.Println("please specify input and parameters flags")
		flag.PrintDefaults()
		os.Exit(1)
	}
	keyValuePairs := strings.Split(parameters, ",")
	if len(keyValuePairs) == 0 {
		fmt.Println("incorrect value of parameters flag")
		flag.PrintDefaults()
		os.Exit(1)
	}
	manifest, err := readAndModifyManifest(manifestPath, keyValuePairs)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println(manifest)
	// TODO(lukasz): send the manifest to kube-api server and verify the desired state
}

func readAndModifyManifest(pathToManifest string, keyValuePairs []string) (string, error) {
	contentRaw, err := ioutil.ReadFile(pathToManifest)
	if err != nil {
		return "", err
	}
	content := fmt.Sprintf("%s", contentRaw)

	for _, keyValuePair := range keyValuePairs {
		kv := strings.Split(keyValuePair, "=")
		if len(kv) != 2 {
			return "", fmt.Errorf("the given key value pair = %v is incorrect, the correct form is key=value", keyValuePair)
		}
		content = strings.Replace(content, kv[0], kv[1], -1)
	}

	return content, nil
}
