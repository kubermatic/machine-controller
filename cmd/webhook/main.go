package main

import (
	"flag"

	"github.com/golang/glog"

	"github.com/kubermatic/machine-controller/pkg/admission"
)

var (
	admissionListenAddress string
	admissionTLSCertPath   string
	admissionTLSKeyPath    string
)

func main() {
	flag.StringVar(&admissionListenAddress, "listen-address", ":9876", "The address on which the MutatingWebhook will listen on")
	flag.StringVar(&admissionTLSCertPath, "tls-cert-path", "/tmp/cert/cert.pem", "The path of the TLS cert for the MutatingWebhook")
	flag.StringVar(&admissionTLSKeyPath, "tls-key-path", "/tmp/cert/key.pem", "The path of the TLS key for the MutatingWebhook")
	flag.Parse()

	s := admission.New(admissionListenAddress)
	glog.Infof("Starting to listen on %s", admissionListenAddress)
	glog.Fatal(s.ListenAndServeTLS(admissionTLSCertPath, admissionTLSKeyPath))
}
