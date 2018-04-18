package main

import (
	"flag"
	"fmt"
	"strings"
	"time"

	verifyhelper "github.com/kubermatic/machine-controller/test/tools/verify"

	"github.com/golang/glog"
)

type arrayFlags string

func (i *arrayFlags) String() string {
	return string(*i)
}

func (i *arrayFlags) Set(value string) error {
	if value == "" {
		return nil
	}
	var direct arrayFlags
	if *i != arrayFlags("") {
		direct = arrayFlags(fmt.Sprintf("%s,%s", i, value))
	} else {
		direct = arrayFlags(value)
	}
	*i = direct

	return nil
}

func main() {
	var manifestPath string
	var parameters arrayFlags
	var kubeConfig string
	var createOnly bool
	var machineReadyCheckTimeout time.Duration

	flag.StringVar(&kubeConfig, "kubeconfig", "", "a path to the kubeconfig.")
	flag.StringVar(&manifestPath, "input", "", "a path to the machine's manifest.")
	flag.Var(&parameters, "parameters", "a list of comma-delimited key value pairs i.e key=value,key1=value2. Can be passed multiple times")
	flag.BoolVar(&createOnly, "createOnly", false, "if the tool should create only but not run deletion")
	flag.DurationVar(&machineReadyCheckTimeout, "machineReadyTimeout", time.Duration(10*time.Minute), "specifies timeout for machine to be ready")
	flag.Parse()

	parametersList := strings.Split(parameters.String(), ",")

	err := verifyhelper.Verify(kubeConfig, manifestPath, parametersList, createOnly, machineReadyCheckTimeout)
	if err != nil {
		glog.Fatalf(err.Error())
	}
}
