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

package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1/migrations"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/util"
	clusterinfo "github.com/kubermatic/machine-controller/pkg/clusterinfo"
	"github.com/kubermatic/machine-controller/pkg/containerruntime"
	machinecontroller "github.com/kubermatic/machine-controller/pkg/controller/machine"
	machinedeploymentcontroller "github.com/kubermatic/machine-controller/pkg/controller/machinedeployment"
	machinesetcontroller "github.com/kubermatic/machine-controller/pkg/controller/machineset"
	"github.com/kubermatic/machine-controller/pkg/controller/nodecsrapprover"
	"github.com/kubermatic/machine-controller/pkg/health"
	machinesv1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/node"
	osmv1alpha1 "k8c.io/operating-system-manager/pkg/crd/osm/v1alpha1"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	masterURL                        string
	kubeconfig                       string
	clusterDNSIPs                    string
	healthProbeAddress               string
	metricsAddress                   string
	profiling                        bool
	name                             string
	joinClusterTimeout               string
	workerCount                      int
	bootstrapTokenServiceAccountName string
	skipEvictionAfter                time.Duration
	caBundleFile                     string

	useOSM bool

	nodeCSRApprover                   bool
	nodeHTTPProxy                     string
	nodeNoProxy                       string
	nodeInsecureRegistries            string
	nodeRegistryMirrors               string
	nodePauseImage                    string
	nodeContainerRuntime              string
	podCIDR                           string
	nodePortRange                     string
	nodeRegistryCredentialsSecret     string
	nodeContainerdRegistryMirrors     = containerruntime.RegistryMirrorsFlags{}
	overrideBootstrapKubeletAPIServer string
)

const (
	defaultLeaderElectionNamespace = "kube-system"
)

// controllerRunOptions holds data that are required to create and run machine controller.
type controllerRunOptions struct {
	// kubeClient a client that knows how to consume kubernetes API.
	kubeClient *kubernetes.Clientset

	// metrics a struct that holds all metrics we want to collect.
	metrics *machinecontroller.MetricsCollection

	// kubeconfigProvider knows how to get cluster information stored under a ConfigMap.
	kubeconfigProvider machinecontroller.KubeconfigProvider

	// name of the controller. When set the controller will only process machines with the label "machine.k8s.io/controller": name.
	name string

	// Name of the ServiceAccount from which the bootstrap token secret will be fetched. A bootstrap token will be created.
	// if this is nil
	bootstrapTokenServiceAccountName *types.NamespacedName

	// prometheusRegisterer is used by the MachineController instance to register its metrics.
	prometheusRegisterer prometheus.Registerer

	// The cfg is used by the migration to conditionally spawn additional clients.
	cfg *restclient.Config

	// The timeout in which machines owned by a MachineSet must join the cluster to avoid being.
	// deleted by the machine-controller
	joinClusterTimeout *time.Duration

	// Will instruct the machine-controller to skip the eviction if the machine deletion is older than skipEvictionAfter.
	skipEvictionAfter time.Duration

	// Enable NodeCSRApprover controller to automatically approve node serving certificate requests.
	nodeCSRApprover bool

	node machinecontroller.NodeSettings

	useOSM bool

	// A port range to reserve for services with NodePort visibility.
	nodePortRange string

	overrideBootstrapKubeletAPIServer string
}

func main() {
	nodeFlags := node.NewFlags(flag.CommandLine)

	klog.InitFlags(nil)
	// This is also being registered in kubevirt.io/kubevirt/pkg/kubecli/kubecli.go so
	// we have to guard it.
	// TODO: Evaluate alternatives to importing the CLI. Generate our own client? Use a dynamic client?
	if flag.Lookup("kubeconfig") == nil {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	}
	if flag.Lookup("master") == nil {
		flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	}
	flag.StringVar(&clusterDNSIPs, "cluster-dns", "10.10.10.10", "Comma-separated list of DNS server IP address.")
	flag.IntVar(&workerCount, "worker-count", 1, "Number of workers to process machines. Using a high number with a lot of machines might cause getting rate-limited from your cloud provider.")
	flag.StringVar(&healthProbeAddress, "health-probe-address", "127.0.0.1:8085", "The address on which the liveness check on /healthz and readiness check on /readyz will be available")
	flag.StringVar(&metricsAddress, "metrics-address", "127.0.0.1:8080", "The address on which Prometheus metrics will be available under /metrics")
	flag.StringVar(&name, "name", "", "When set, the controller will only process machines with the label \"machine.k8s.io/controller\": name")
	flag.StringVar(&joinClusterTimeout, "join-cluster-timeout", "", "when set, machines that have an owner and do not join the cluster within the configured duration will be deleted, so the owner re-creats them")
	flag.StringVar(&bootstrapTokenServiceAccountName, "bootstrap-token-service-account-name", "", "When set use the service account token from this SA as bootstrap token instead of creating a temporary one. Passed in namespace/name format")
	flag.BoolVar(&profiling, "enable-profiling", false, "when set, enables the endpoints on the http server under /debug/pprof/")
	flag.DurationVar(&skipEvictionAfter, "skip-eviction-after", 2*time.Hour, "Skips the eviction if a machine is not gone after the specified duration.")
	flag.StringVar(&nodeHTTPProxy, "node-http-proxy", "", "If set, it configures the 'HTTP_PROXY' & 'HTTPS_PROXY' environment variable on the nodes.")
	flag.StringVar(&nodeNoProxy, "node-no-proxy", ".svc,.cluster.local,localhost,127.0.0.1", "If set, it configures the 'NO_PROXY' environment variable on the nodes.")
	flag.StringVar(&nodeInsecureRegistries, "node-insecure-registries", "", "Comma separated list of registries which should be configured as insecure on the container runtime")
	flag.StringVar(&nodeRegistryMirrors, "node-registry-mirrors", "", "Comma separated list of Docker image mirrors")
	flag.StringVar(&nodePauseImage, "node-pause-image", "", "Image for the pause container including tag. If not set, the kubelet default will be used: https://kubernetes.io/docs/reference/command-line-tools-reference/kubelet/")
	flag.String("node-kubelet-repository", "quay.io/kubermatic/kubelet", "[NO-OP] Repository for the kubelet container. Has no effects.")
	flag.StringVar(&nodeContainerRuntime, "node-container-runtime", "docker", "container-runtime to deploy")
	flag.Var(&nodeContainerdRegistryMirrors, "node-containerd-registry-mirrors", "Configure registry mirrors endpoints. Can be used multiple times to specify multiple mirrors")
	flag.StringVar(&caBundleFile, "ca-bundle", "", "path to a file containing all PEM-encoded CA certificates (will be used instead of the host's certificates if set)")
	flag.BoolVar(&nodeCSRApprover, "node-csr-approver", true, "Enable NodeCSRApprover controller to automatically approve node serving certificate requests")
	flag.StringVar(&podCIDR, "pod-cidr", "172.25.0.0/16", "WARNING: flag is unused, kept only for backwards compatibility")
	flag.StringVar(&nodePortRange, "node-port-range", "30000-32767", "A port range to reserve for services with NodePort visibility")
	flag.StringVar(&nodeRegistryCredentialsSecret, "node-registry-credentials-secret", "", "A Secret object reference, that contains auth info for image registry in namespace/secret-name form, example: kube-system/registry-credentials. See doc at https://github.com/kubermaric/machine-controller/blob/master/docs/registry-authentication.md")
	flag.BoolVar(&useOSM, "use-osm", false, "use osm controller for node bootstrap")
	flag.StringVar(&overrideBootstrapKubeletAPIServer, "override-bootstrap-kubelet-apiserver", "", "Override for the API server address used in worker nodes bootstrap-kubelet.conf")

	flag.Parse()
	kubeconfig = flag.Lookup("kubeconfig").Value.(flag.Getter).Get().(string)
	masterURL = flag.Lookup("master").Value.(flag.Getter).Get().(string)

	clusterDNSIPs, err := parseClusterDNSIPs(clusterDNSIPs)
	if err != nil {
		klog.Fatalf("invalid cluster dns specified: %v", err)
	}

	var parsedJoinClusterTimeout *time.Duration
	if joinClusterTimeout != "" {
		parsedJoinClusterTimeoutLiteral, err := time.ParseDuration(joinClusterTimeout)
		parsedJoinClusterTimeout = &parsedJoinClusterTimeoutLiteral
		if err != nil {
			klog.Fatalf("failed to parse join-cluster-timeout as duration: %v", err)
		}
	}

	// Needed for migrations
	if err := machinesv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		klog.Fatalf("failed to add machinesv1alpha1 api to scheme: %v", err)
	}
	if err := apiextensionsv1.AddToScheme(scheme.Scheme); err != nil {
		klog.Fatalf("failed to add apiextensionsv1 api to scheme: %v", err)
	}
	if err := clusterv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		klog.Fatalf("failed to add clusterv1alpha1 api to scheme: %v", err)
	}

	// needed for OSM
	if err := osmv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		klog.Fatalf("failed to add osmv1alpha1 api to scheme: %v", err)
	}

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("error building kubeconfig: %v", err)
	}

	if caBundleFile != "" {
		if err := util.SetCABundleFile(caBundleFile); err != nil {
			klog.Fatalf("-ca-bundle is invalid: %v", err)
		}
	}

	// rest.Config has no DeepCopy() that returns another rest.Config, thus
	// we simply build it twice
	// We need a dedicated one for machines because we want to increase the
	// QPS and Burst config there
	machineCfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("error building kubeconfig for machines: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("error building kubernetes clientset for kubeClient: %v", err)
	}
	kubeconfigProvider := clusterinfo.New(cfg, kubeClient)

	ctrlMetrics := machinecontroller.NewMachineControllerMetrics()
	ctrlMetrics.MustRegister(metrics.Registry)

	containerRuntimeOpts := containerruntime.Opts{
		ContainerRuntime:          nodeContainerRuntime,
		ContainerdRegistryMirrors: nodeContainerdRegistryMirrors,
		InsecureRegistries:        nodeInsecureRegistries,
		PauseImage:                nodePauseImage,
		RegistryMirrors:           nodeRegistryMirrors,
		RegistryCredentialsSecret: nodeRegistryCredentialsSecret,
	}
	containerRuntimeConfig, err := containerruntime.BuildConfig(containerRuntimeOpts)
	if err != nil {
		klog.Fatalf("failed to generate container runtime config: %v", err)
	}

	runOptions := controllerRunOptions{
		kubeClient:           kubeClient,
		kubeconfigProvider:   kubeconfigProvider,
		name:                 name,
		cfg:                  machineCfg,
		metrics:              ctrlMetrics,
		prometheusRegisterer: metrics.Registry,
		skipEvictionAfter:    skipEvictionAfter,
		nodeCSRApprover:      nodeCSRApprover,
		node: machinecontroller.NodeSettings{
			ClusterDNSIPs:                clusterDNSIPs,
			HTTPProxy:                    nodeHTTPProxy,
			NoProxy:                      nodeNoProxy,
			PauseImage:                   nodePauseImage,
			RegistryCredentialsSecretRef: nodeRegistryCredentialsSecret,
			ContainerRuntime:             containerRuntimeConfig,
		},
		useOSM:                            useOSM,
		nodePortRange:                     nodePortRange,
		overrideBootstrapKubeletAPIServer: overrideBootstrapKubeletAPIServer,
	}

	if err := nodeFlags.UpdateNodeSettings(&runOptions.node); err != nil {
		klog.Fatalf("failed to update nodesettings: %v", err)
	}

	if parsedJoinClusterTimeout != nil {
		runOptions.joinClusterTimeout = parsedJoinClusterTimeout
	}

	if bootstrapTokenServiceAccountName != "" {
		flagParts := strings.Split(bootstrapTokenServiceAccountName, "/")
		if flagPartsLen := len(flagParts); flagPartsLen != 2 {
			klog.Fatalf("Splitting the bootstrap-token-service-account-name flag value in '/' returned %d parts, expected exactly two", flagPartsLen)
		}
		runOptions.bootstrapTokenServiceAccountName = &types.NamespacedName{Namespace: flagParts[0], Name: flagParts[1]}
	}

	ctx := signals.SetupSignalHandler()
	go func() {
		<-ctx.Done()
		klog.Info("caught signal, shutting down...")
	}()

	mgr, err := createManager(5*time.Minute, runOptions)
	if err != nil {
		klog.Fatalf("failed to create runtime manager: %v", err)
	}

	if err := mgr.Start(ctx); err != nil {
		klog.Errorf("failed to start kubebuilder manager: %v", err)
	}
}

func createManager(syncPeriod time.Duration, options controllerRunOptions) (manager.Manager, error) {
	mgr, err := manager.New(options.cfg, manager.Options{
		SyncPeriod:              &syncPeriod,
		LeaderElection:          true,
		LeaderElectionID:        "machine-controller",
		LeaderElectionNamespace: defaultLeaderElectionNamespace,
		HealthProbeBindAddress:  healthProbeAddress,
		MetricsBindAddress:      metricsAddress,
	})
	if err != nil {
		return nil, fmt.Errorf("error building ctrlruntime manager: %w", err)
	}

	if err := mgr.AddReadyzCheck("alive", healthz.Ping); err != nil {
		return nil, fmt.Errorf("failed to add readiness check: %w", err)
	}

	if err := mgr.AddHealthzCheck("kubeconfig", health.KubeconfigAvailable(options.kubeconfigProvider)); err != nil {
		return nil, fmt.Errorf("failed to add health check: %w", err)
	}

	if err := mgr.AddHealthzCheck("apiserver-connection", health.ApiserverReachable(options.kubeClient)); err != nil {
		return nil, fmt.Errorf("failed to add health check: %w", err)
	}

	if profiling {
		m := http.NewServeMux()
		m.HandleFunc("/", pprof.Index)
		m.HandleFunc("/cmdline", pprof.Cmdline)
		m.HandleFunc("/profile", pprof.Profile)
		m.HandleFunc("/symbol", pprof.Symbol)
		m.HandleFunc("/trace", pprof.Trace)

		if err := mgr.AddMetricsExtraHandler("/debug/pprof/", m); err != nil {
			return nil, fmt.Errorf("failed to add pprof http handlers: %w", err)
		}
	}

	if err := mgr.Add(&controllerBootstrap{
		mgr: mgr,
		opt: options,
	}); err != nil {
		return nil, fmt.Errorf("failed to add bootstrap runnable: %w", err)
	}

	return mgr, nil
}

type controllerBootstrap struct {
	mgr manager.Manager
	opt controllerRunOptions
}

// NeedLeaderElection implements manager.LeaderElectionRunnable.
func (bs *controllerBootstrap) NeedLeaderElection() bool {
	return true
}

// Start is called when the leader election succeeded and is meant to
// coordinate running the migrations first, then starting the controllers.
// Start is part of manager.Runnable.
func (bs *controllerBootstrap) Start(ctx context.Context) error {
	client := bs.mgr.GetClient()

	providerData := &cloudprovidertypes.ProviderData{
		Ctx:    ctx,
		Update: cloudprovidertypes.GetMachineUpdater(ctx, client),
		Client: client,
	}

	// Migrate MachinesV1Alpha1Machine to ClusterV1Alpha1Machine.
	if err := migrations.MigrateMachinesv1Alpha1MachineToClusterv1Alpha1MachineIfNecessary(ctx, client, bs.opt.kubeClient, providerData); err != nil {
		return fmt.Errorf("migration to clusterv1alpha1 failed: %w", err)
	}

	// Migrate providerConfig field to providerSpec field.
	if err := migrations.MigrateProviderConfigToProviderSpecIfNecesary(ctx, bs.opt.cfg, client); err != nil {
		return fmt.Errorf("migration of providerConfig field to providerSpec field failed: %w", err)
	}

	machineCollector := machinecontroller.NewMachineCollector(ctx, bs.mgr.GetClient())
	metrics.Registry.MustRegister(machineCollector)

	if err := machinecontroller.Add(
		ctx,
		bs.mgr,
		bs.opt.kubeClient,
		workerCount,
		bs.opt.metrics,
		bs.opt.kubeconfigProvider,
		providerData,
		bs.opt.joinClusterTimeout,
		bs.opt.name,
		bs.opt.bootstrapTokenServiceAccountName,
		bs.opt.skipEvictionAfter,
		bs.opt.node,
		bs.opt.useOSM,
		bs.opt.nodePortRange,
		bs.opt.overrideBootstrapKubeletAPIServer,
	); err != nil {
		return fmt.Errorf("failed to add Machine controller to manager: %w", err)
	}

	if err := machinesetcontroller.Add(bs.mgr); err != nil {
		return fmt.Errorf("failed to add MachineSet controller to manager: %w", err)
	}

	if err := machinedeploymentcontroller.Add(bs.mgr); err != nil {
		return fmt.Errorf("failed to add MachineDeployment controller to manager: %w", err)
	}

	if bs.opt.nodeCSRApprover {
		if err := nodecsrapprover.Add(bs.mgr); err != nil {
			return fmt.Errorf("failed to add NodeCSRApprover controller to manager: %w", err)
		}
	}

	klog.Info("machine controller startup complete")

	return nil
}

func parseClusterDNSIPs(s string) ([]net.IP, error) {
	var ips []net.IP
	sips := strings.Split(s, ",")
	for _, sip := range sips {
		ip := net.ParseIP(strings.TrimSpace(sip))
		if ip == nil {
			return nil, fmt.Errorf("unable to parse ip %s", sip)
		}
		ips = append(ips, ip)
	}
	return ips, nil
}
