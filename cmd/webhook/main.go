package main

import (
	"flag"

	"github.com/golang/glog"

	"github.com/kubermatic/machine-controller/pkg/admission"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	masterURL              string
	kubeconfig             string
	admissionListenAddress string
	admissionTLSCertPath   string
	admissionTLSKeyPath    string
)

func main() {
	if flag.Lookup("kubeconfig") == nil {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	}
	if flag.Lookup("master") == nil {
		flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	}
	flag.StringVar(&admissionListenAddress, "listen-address", ":9876", "The address on which the MutatingWebhook will listen on")
	flag.StringVar(&admissionTLSCertPath, "tls-cert-path", "/tmp/cert/cert.pem", "The path of the TLS cert for the MutatingWebhook")
	flag.StringVar(&admissionTLSKeyPath, "tls-key-path", "/tmp/cert/key.pem", "The path of the TLS key for the MutatingWebhook")
	flag.Parse()
	kubeconfig = flag.Lookup("kubeconfig").Value.(flag.Getter).Get().(string)
	masterURL = flag.Lookup("master").Value.(flag.Getter).Get().(string)

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		glog.Fatalf("error building kubeconfig: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("error building kubernetes clientset for kubeClient: %v", err)
	}

	s := admission.New(admissionListenAddress, kubeClient)
	if err := s.ListenAndServeTLS(admissionTLSCertPath, admissionTLSKeyPath); err != nil {
		glog.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			glog.Fatalf("Failed to shutdown server: %v", err)
		}
	}()
	glog.Infof("Listening on %s", admissionListenAddress)
	select {}
}
