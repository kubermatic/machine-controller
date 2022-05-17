/*
Copyright 2019 The Machine Controller Authors.

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

package util

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/google/uuid"

	"k8s.io/klog"
)

const defaultClientTimeout = 15 * time.Second

var (
	// CABundle is set globally once by the main() function
	// and is used to overwrite the default set of CA certificates
	// loaded from the host system/pod.
	CABundle *x509.CertPool
)

// SetCABundleFile reads a PEM-encoded file and replaces the current
// global CABundle with a new one. The file must contain at least one
// valid certificate.
func SetCABundleFile(filename string) error {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	CABundle = x509.NewCertPool()
	if !CABundle.AppendCertsFromPEM(content) {
		return errors.New("file does not contain valid PEM-encoded certificates")
	}

	return nil
}

type HTTPClientConfig struct {
	// LogPrefix is prepended to request/response logs
	LogPrefix string
	// Global timeout used by the client
	Timeout time.Duration
}

// New return a custom HTTP client that allows for logging
// HTTP request and response information.
func (c HTTPClientConfig) New() http.Client {
	timeout := c.Timeout
	// Enforce a global timeout
	if timeout <= 0 {
		timeout = defaultClientTimeout
	}

	return http.Client{
		Transport: &LogRoundTripper{
			logPrefix: c.LogPrefix,
			rt: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: CABundle,
				},
			},
		},
		Timeout: timeout,
	}
}

// LogRoundTripper is used to log information about requests and responses that
// may be useful for debugging purposes.
// Note that setting log level >5 results in full dumps of requests and
// responses, including sensitive invormation (e.g. Authorization header).
type LogRoundTripper struct {
	logPrefix string
	rt        http.RoundTripper
}

func (lrt *LogRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	var log []byte
	var err error
	// Generate unique ID to correlate requests and responses
	id := uuid.New()
	switch {
	case bool(klog.V(6)):
		log, err = httputil.DumpRequest(request, true)
		if err != nil {
			klog.Warningf("Error occurred while dumping request: %v", err)
		}
	case bool(klog.V(5)):
		log, err = httputil.DumpRequest(request, false)
		if err != nil {
			klog.Warningf("Error occurred while dumping request: %v", err)
		}
	default:
		var b bytes.Buffer
		fmt.Fprintf(&b, "%s %s HTTP/%d.%d", valueOrDefault(request.Method, "GET"),
			request.URL.RequestURI(), request.ProtoMajor, request.ProtoMinor)
		log = b.Bytes()
	}
	klog.V(1).Infof("%s request sent [%s]: %s\n", lrt.logPrefix, id.String(), string(log))

	response, err := lrt.rt.RoundTrip(request)
	if response == nil {
		return nil, err
	}

	switch {
	case bool(klog.V(6)):
		log, err = httputil.DumpResponse(response, true)
		if err != nil {
			klog.Warningf("Error occurred while dumping response: %v", err)
		}
	case bool(klog.V(5)):
		log, err = httputil.DumpResponse(response, false)
		if err != nil {
			klog.Warningf("Error occurred while dumping response: %v", err)
		}
	default:
		var b bytes.Buffer
		fmt.Fprintf(&b, "HTTP/%d.%d %03d", response.ProtoMajor, response.ProtoMinor, response.StatusCode)
		log = b.Bytes()
	}
	klog.V(1).Infof("%s request received [%s]: %s\n", lrt.logPrefix, id.String(), string(log))

	return response, nil
}

// Return value if nonempty, def otherwise.
func valueOrDefault(value, def string) string {
	if value != "" {
		return value
	}
	return def
}
