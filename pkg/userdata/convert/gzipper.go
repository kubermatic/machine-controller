package convert

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net"

	"github.com/kubermatic/machine-controller/pkg/userdata/cloud"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

func NewGzip(p Provider) *Gzip {
	return &Gzip{p: p}
}

type Gzip struct {
	p Provider
}

func (g *Gzip) UserData(spec clusterv1alpha1.MachineSpec, kubeconfig *clientcmdapi.Config, ccProvider cloud.ConfigProvider, clusterDNSIPs []net.IP) (string, error) {
	before, err := g.p.UserData(spec, kubeconfig, ccProvider, clusterDNSIPs)
	if err != nil {
		return "", err
	}

	out, err := GzipString(before)
	if err != nil {
		return "", fmt.Errorf("failed to gzip userdata: %v", err)
	}

	return out, nil
}

func GzipString(s string) (string, error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)

	if _, err := gz.Write([]byte(s)); err != nil {
		return "", err
	}

	if err := gz.Flush(); err != nil {
		return "", err
	}

	if err := gz.Close(); err != nil {
		return "", err
	}

	return b.String(), nil
}
