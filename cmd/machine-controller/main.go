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
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"

	"github.com/docker/distribution/reference"
	"github.com/heptiolabs/healthcheck"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1/migrations"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/clusterinfo"
	machinecontroller "github.com/kubermatic/machine-controller/pkg/controller/machine"
	machinedeploymentcontroller "github.com/kubermatic/machine-controller/pkg/controller/machinedeployment"
	machinesetcontroller "github.com/kubermatic/machine-controller/pkg/controller/machineset"
	"github.com/kubermatic/machine-controller/pkg/controller/nodecsrapprover"
	machinehealth "github.com/kubermatic/machine-controller/pkg/health"
	machinesv1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/signals"

	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/klog"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	masterURL                        string
	kubeconfig                       string
	clusterDNSIPs                    string
	listenAddress                    string
	profiling                        bool
	name                             string
	joinClusterTimeout               string
	workerCount                      int
	externalCloudProvider            bool
	bootstrapTokenServiceAccountName string
	skipEvictionAfter                time.Duration
	nodeCSRApprover                  bool

	nodeHTTPProxy           string
	nodeNoProxy             string
	nodeInsecureRegistries  string
	nodeRegistryMirrors     string
	nodePauseImage          string
	nodeHyperkubeImage      string
	nodeKubeletRepository   string
	nodeKubeletFeatureGates string
)

const (
	defaultLeaderElectionNamespace     = "kube-system"
	defaultLeaderElectionLeaseDuration = 15 * time.Second
	defaultLeaderElectionRenewDeadline = 10 * time.Second
	defaultLeaderElectionRetryPeriod   = 2 * time.Second
)

// controllerRunOptions holds data that are required to create and run machine controller
type controllerRunOptions struct {
	// kubeClient a client that knows how to consume kubernetes API
	kubeClient *kubernetes.Clientset

	// metrics a struct that holds all metrics we want to collect
	metrics *machinecontroller.MetricsCollection

	// kubeconfigProvider knows how to get cluster information stored under a ConfigMap
	kubeconfigProvider machinecontroller.KubeconfigProvider

	// name of the controller. When set the controller will only process machines with the label "machine.k8s.io/controller": name
	name string

	// Name of the ServiceAccount from which the bootstrap token secret will be fetched. A bootstrap token will be created
	// if this is nil
	bootstrapTokenServiceAccountName *types.NamespacedName

	// parentCtx carries a cancellation signal
	parentCtx context.Context

	// parentCtxDone allows you to close parentCtx
	// since context can form a tree-like structure it seems to be odd to pass done function of a parent
	// and allow dependant function to close the parent.
	// it should be the other way around i.e. derive a new context from the parent
	parentCtxDone context.CancelFunc

	// prometheusRegisterer is used by the MachineController instance to register its metrics
	prometheusRegisterer prometheus.Registerer

	// The cfg is used by the migration to conditionally spawn additional clients
	cfg *restclient.Config

	// The timeout in which machines owned by a MachineSet must join the cluster to avoid being
	// deleted by the machine-controller
	joinClusterTimeout *time.Duration

	// Flag to initialize kubelets with --cloud-provider=external
	externalCloudProvider bool

	// Will instruct the machine-controller to skip the eviction if the machine deletion is older than skipEvictionAfter
	skipEvictionAfter time.Duration

	// Enable NodeCSRApprover controller to automatically approve node serving certificate requests.
	nodeCSRApprover bool

	node machinecontroller.NodeSettings
}

func main() {
	klog.InitFlags(nil)
	// This is also being registered in kubevirt.io/kubevirt/pkg/kubecli/kubecli.go so
	// we have to guard it
	//TODO: Evaluate alternatives to importing the CLI. Generate our own client? Use a dynamic client?
	if flag.Lookup("kubeconfig") == nil {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	}
	if flag.Lookup("master") == nil {
		flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	}
	flag.StringVar(&clusterDNSIPs, "cluster-dns", "10.10.10.10", "Comma-separated list of DNS server IP address.")
	flag.IntVar(&workerCount, "worker-count", 5, "Number of workers to process machines. Using a high number with a lot of machines might cause getting rate-limited from your cloud provider.")
	flag.StringVar(&listenAddress, "internal-listen-address", "127.0.0.1:8085", "The address on which the http server will listen on. The server exposes metrics on /metrics, liveness check on /live and readiness check on /ready")
	flag.StringVar(&name, "name", "", "When set, the controller will only process machines with the label \"machine.k8s.io/controller\": name")
	flag.StringVar(&joinClusterTimeout, "join-cluster-timeout", "", "when set, machines that have an owner and do not join the cluster within the configured duration will be deleted, so the owner re-creats them")
	flag.StringVar(&bootstrapTokenServiceAccountName, "bootstrap-token-service-account-name", "", "When set use the service account token from this SA as bootstrap token instead of creating a temporary one. Passed in namespace/name format")
	flag.BoolVar(&profiling, "enable-profiling", false, "when set, enables the endpoints on the http server under /debug/pprof/")
	flag.BoolVar(&externalCloudProvider, "external-cloud-provider", false, "when set, kubelets will receive --cloud-provider=external flag")
	flag.DurationVar(&skipEvictionAfter, "skip-eviction-after", 2*time.Hour, "Skips the eviction if a machine is not gone after the specified duration.")
	flag.StringVar(&nodeHTTPProxy, "node-http-proxy", "", "If set, it configures the 'HTTP_PROXY' & 'HTTPS_PROXY' environment variable on the nodes.")
	flag.StringVar(&nodeNoProxy, "node-no-proxy", ".svc,.cluster.local,localhost,127.0.0.1", "If set, it configures the 'NO_PROXY' environment variable on the nodes.")
	flag.StringVar(&nodeInsecureRegistries, "node-insecure-registries", "", "Comma separated list of registries which should be configured as insecure on the container runtime")
	flag.StringVar(&nodeRegistryMirrors, "node-registry-mirrors", "", "Comma separated list of Docker image mirrors")
	flag.StringVar(&nodePauseImage, "node-pause-image", "", "Image for the pause container including tag. If not set, the kubelet default will be used: https://kubernetes.io/docs/reference/command-line-tools-reference/kubelet/")
	flag.StringVar(&nodeHyperkubeImage, "node-hyperkube-image", "k8s.gcr.io/hyperkube-amd64", "Image for the hyperkube container excluding tag. Only has effect on CoreOS Container Linux and Flatcar Linux, and for kubernetes < 1.18.")
	flag.StringVar(&nodeKubeletRepository, "node-kubelet-repository", "quay.io/poseidon/kubelet", "Repository for the kubelet container. Only has effect on Flatcar Linux, and for kubernetes >= 1.18.")
	flag.StringVar(&nodeKubeletFeatureGates, "node-kubelet-feature-gates", "RotateKubeletServerCertificate=true", "Feature gates to set on the kubelet. Default: RotateKubeletServerCertificate=true")
	flag.BoolVar(&nodeCSRApprover, "node-csr-approver", false, "Enable NodeCSRApprover controller to automatically approve node serving certificate requests.")

	flag.Parse()
	kubeconfig = flag.Lookup("kubeconfig").Value.(flag.Getter).Get().(string)
	masterURL = flag.Lookup("master").Value.(flag.Getter).Get().(string)

	clusterDNSIPs, err := parseClusterDNSIPs(clusterDNSIPs)
	if err != nil {
		klog.Fatalf("invalid cluster dns specified: %v", err)
	}

	kubeletFeatureGates, err := parseKubeletFeatureGates(nodeKubeletFeatureGates)
	if err != nil {
		klog.Fatalf("invalid kubelet feature gates specified: %v", err)
	}

	var parsedJoinClusterTimeout *time.Duration
	if joinClusterTimeout != "" {
		parsedJoinClusterTimeoutLiteral, err := time.ParseDuration(joinClusterTimeout)
		parsedJoinClusterTimeout = &parsedJoinClusterTimeoutLiteral
		if err != nil {
			klog.Fatalf("failed to parse join-cluster-timeout as duration: %v", err)
		}
	}

	stopCh := signals.SetupSignalHandler()

	// Needed for migrations
	if err := machinesv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		klog.Fatalf("failed to add machinesv1alpha1 api to scheme: %v", err)
	}
	if err := apiextensionsv1beta1.AddToScheme(scheme.Scheme); err != nil {
		klog.Fatalf("failed to add apiextensionv1beta1 api to scheme: %v", err)
	}
	if err := clusterv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		klog.Fatalf("failed to add clusterv1alpha1 api to scheme: %v", err)
	}

	// Check if the hyperkube image has a tag set
	hyperkubeImageRef, err := reference.Parse(nodeHyperkubeImage)
	if err != nil {
		klog.Fatalf("failed to parse -node-hyperkube-image %s: %v", nodeHyperkubeImage, err)
	}
	if _, ok := hyperkubeImageRef.(reference.NamedTagged); ok {
		klog.Fatalf("-node-hyperkube-image must not contain a tag. The tag will be dynamically set for each Machine.")
	}

	// Check if the kubelet image has a tag set
	kubeletRepoRef, err := reference.Parse(nodeKubeletRepository)
	if err != nil {
		klog.Fatalf("failed to parse -node-hyperkube-image %s: %v", nodeHyperkubeImage, err)
	}
	if _, ok := kubeletRepoRef.(reference.NamedTagged); ok {
		klog.Fatalf("-node-kubelet-image must not contain a tag. The tag will be dynamically set for each Machine.")
	}

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("error building kubeconfig: %v", err)
	}

	// rest.Config has no DeepCopy() that returns another rest.Config, thus
	// we simply build it twice
	// We need a dedicated one for machines because we want to increate the
	// QPS and Burst config there
	machineCfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("error building kubeconfig for machines: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("error building kubernetes clientset for kubeClient: %v", err)
	}

	ctrlruntimeClient, err := ctrlruntimeclient.New(cfg, ctrlruntimeclient.Options{})
	if err != nil {
		klog.Fatalf("error building ctrlruntime client: %v", err)
	}

	prometheusRegistry := prometheus.DefaultRegisterer

	kubeconfigProvider := clusterinfo.New(cfg, kubeClient)
	runOptions := controllerRunOptions{
		kubeClient: kubeClient,
		metrics:    machinecontroller.NewMachineControllerMetrics(),

		kubeconfigProvider:    kubeconfigProvider,
		name:                  name,
		prometheusRegisterer:  prometheusRegistry,
		cfg:                   machineCfg,
		externalCloudProvider: externalCloudProvider,
		skipEvictionAfter:     skipEvictionAfter,
		nodeCSRApprover:       nodeCSRApprover,
		node: machinecontroller.NodeSettings{
			ClusterDNSIPs:       clusterDNSIPs,
			HTTPProxy:           nodeHTTPProxy,
			NoProxy:             nodeNoProxy,
			HyperkubeImage:      nodeHyperkubeImage,
			KubeletRepository:   nodeKubeletRepository,
			KubeletFeatureGates: kubeletFeatureGates,
			PauseImage:          nodePauseImage,
		},
	}
	if parsedJoinClusterTimeout != nil {
		runOptions.joinClusterTimeout = parsedJoinClusterTimeout
	}

	for _, registry := range strings.Split(nodeInsecureRegistries, ",") {
		if trimmedRegistry := strings.TrimSpace(registry); trimmedRegistry != "" {
			runOptions.node.InsecureRegistries = append(runOptions.node.InsecureRegistries, trimmedRegistry)
		}
	}

	for _, mirror := range strings.Split(nodeRegistryMirrors, ",") {
		if trimmedMirror := strings.TrimSpace(mirror); trimmedMirror != "" {
			runOptions.node.RegistryMirrors = append(runOptions.node.RegistryMirrors, trimmedMirror)
		}
	}

	if bootstrapTokenServiceAccountName != "" {
		flagParts := strings.Split(bootstrapTokenServiceAccountName, "/")
		if flagPartsLen := len(flagParts); flagPartsLen != 2 {
			klog.Fatalf("Splitting the bootstrap-token-service-account-name flag value in '/' returned %d parts, expected exactly two", flagPartsLen)
		}
		runOptions.bootstrapTokenServiceAccountName = &types.NamespacedName{Namespace: flagParts[0], Name: flagParts[1]}
	}

	ctx, ctxDone := context.WithCancel(context.Background())
	var g run.Group
	{
		prometheusRegistry.MustRegister(machinecontroller.NewMachineCollector(ctx, ctrlruntimeClient))

		s := createUtilHTTPServer(kubeClient, kubeconfigProvider, prometheus.DefaultGatherer)
		g.Add(func() error {
			return s.ListenAndServe()
		}, func(err error) {
			klog.Warningf("shutting down HTTP server due to: %s", err)
			srvCtx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			if err = s.Shutdown(srvCtx); err != nil {
				klog.Errorf("failed to shutdown HTTP server: %s", err)
			}
		})
	}
	{
		g.Add(func() error {
			select {
			case <-stopCh:
				return errors.New("user requested to stop the application")
			case <-ctx.Done():
				return errors.New("parent context has been closed - propagating the request")
			}
		}, func(err error) {
			ctxDone()
		})

	}
	{
		g.Add(func() error {
			runOptions.parentCtx = ctx
			runOptions.parentCtxDone = ctxDone
			return startControllerViaLeaderElection(runOptions)
		}, func(err error) {
			ctxDone()
		})
	}

	klog.Info(g.Run())
}

// startControllerViaLeaderElection starts machine controller only if a proper lock was acquired.
// This essentially means that we can have multiple instances and at the same time only one is operational.
// The program terminates when the leadership was lost.
func startControllerViaLeaderElection(runOptions controllerRunOptions) error {
	mgrSyncPeriod := 5 * time.Minute
	mgr, err := manager.New(runOptions.cfg, manager.Options{SyncPeriod: &mgrSyncPeriod})
	if err != nil {
		klog.Errorf("failed to create manager: %v", err)
		runOptions.parentCtxDone()
		return err
	}

	id, err := os.Hostname()
	if err != nil {
		klog.Fatalf("error getting hostname: %s", err.Error())
	}
	// add a seed to the id, so that two processes on the same host don't accidentally both become active
	id = id + "_" + string(uuid.NewUUID())

	// add worker name to the election lock name to prevent conflicts between controllers handling different worker labels
	leaderName := strings.Replace(machinecontroller.ControllerName, "_", "-", -1)
	if runOptions.name != "" {
		leaderName = runOptions.name + "-" + leaderName
	}

	rl := resourcelock.EndpointsLock{
		EndpointsMeta: metav1.ObjectMeta{
			Namespace: defaultLeaderElectionNamespace,
			Name:      leaderName,
		},
		Client: runOptions.kubeClient.CoreV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity:      id + fmt.Sprintf("-%s", leaderName),
			EventRecorder: mgr.GetEventRecorderFor("machine_controller_leader_election"),
		},
	}

	// I think this might be a bit paranoid but the fact the there is no way
	// to stop the leader election library might cause synchronization issues.
	// imagine that a user wants to shutdown the app but since there is no way of telling the library to stop it will eventually run `runController` method
	// and bad things can happen - the fact it works at the moment doesn't mean it will in the future
	runController := func(ctx context.Context) {

		providerData := &cloudprovidertypes.ProviderData{
			Ctx:    ctx,
			Update: cloudprovidertypes.GetMachineUpdater(ctx, mgr.GetClient()),
			Client: mgr.GetClient(),
		}
		// We must start the manager before we add any of the controllers, because
		// the migrations must run before the controllers but need the mgrs client.
		go func() {
			if err := mgr.Start(runOptions.parentCtx.Done()); err != nil {
				klog.Errorf("failed to start kubebuilder manager: %v", err)
				runOptions.parentCtxDone()
			}
		}()
		cacheSyncContext, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if synced := mgr.GetCache().WaitForCacheSync(cacheSyncContext.Done()); !synced {
			klog.Error("Timed out waiting for cache to sync")
			return
		}

		// Migrate MachinesV1Alpha1Machine to ClusterV1Alpha1Machine
		if err := migrations.MigrateMachinesv1Alpha1MachineToClusterv1Alpha1MachineIfNecessary(ctx, mgr.GetClient(), runOptions.kubeClient, providerData); err != nil {
			klog.Errorf("Migration to clusterv1alpha1 failed: %v", err)
			runOptions.parentCtxDone()
			return
		}

		// Migrate providerConfig field to providerSpec field
		if err := migrations.MigrateProviderConfigToProviderSpecIfNecesary(ctx, runOptions.cfg, mgr.GetClient()); err != nil {
			klog.Errorf("Migration of providerConfig field to providerSpec field failed: %v", err)
			runOptions.parentCtxDone()
			return
		}

		if err := machinecontroller.Add(
			ctx,
			mgr,
			runOptions.kubeClient,
			workerCount,
			runOptions.metrics,
			runOptions.prometheusRegisterer,
			runOptions.kubeconfigProvider,
			providerData,
			runOptions.joinClusterTimeout,
			runOptions.externalCloudProvider,
			runOptions.name,
			runOptions.bootstrapTokenServiceAccountName,
			runOptions.skipEvictionAfter,
			runOptions.node,
		); err != nil {
			klog.Errorf("failed to add Machine controller to manager: %v", err)
			runOptions.parentCtxDone()
			return
		}
		if err := machinesetcontroller.Add(mgr); err != nil {
			klog.Errorf("failed to add MachineSet controller to manager: %v", err)
			runOptions.parentCtxDone()
			return
		}
		if err := machinedeploymentcontroller.Add(mgr); err != nil {
			klog.Errorf("failed to add MachineDeployment controller to manager: %v", err)
			runOptions.parentCtxDone()
			return
		}
		if runOptions.nodeCSRApprover {
			if err := nodecsrapprover.Add(mgr); err != nil {
				klog.Errorf("failed to add NodeCSRApprover controller to manager: %v", err)
				runOptions.parentCtxDone()
				return
			}
		}

		klog.Info("machine controller startup complete")
	}

	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          &rl,
		LeaseDuration: defaultLeaderElectionLeaseDuration,
		RenewDeadline: defaultLeaderElectionRenewDeadline,
		RetryPeriod:   defaultLeaderElectionRetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: runController,
			OnStoppedLeading: func() {
				runOptions.parentCtxDone()
			},
		},
	})
	if err != nil {
		return err
	}
	go le.Run(runOptions.parentCtx)

	<-runOptions.parentCtx.Done()
	klog.Info("machine controller has been successfully stopped")
	return nil
}

// createUtilHTTPServer creates a new HTTP server
func createUtilHTTPServer(kubeClient kubernetes.Interface, kubeconfigProvider machinecontroller.KubeconfigProvider, prometheusGatherer prometheus.Gatherer) *http.Server {
	health := healthcheck.NewHandler()
	health.AddReadinessCheck("apiserver-connection", machinehealth.ApiserverReachable(kubeClient))

	for name, c := range readinessChecks(kubeconfigProvider) {
		health.AddReadinessCheck(name, c)
	}

	m := http.NewServeMux()
	m.Handle("/metrics", promhttp.HandlerFor(prometheusGatherer, promhttp.HandlerOpts{}))
	m.Handle("/live", http.HandlerFunc(health.LiveEndpoint))
	m.Handle("/ready", http.HandlerFunc(health.ReadyEndpoint))
	if profiling {
		m.HandleFunc("/debug/pprof/", pprof.Index)
		m.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		m.HandleFunc("/debug/pprof/profile", pprof.Profile)
		m.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		m.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	return &http.Server{
		Addr:         listenAddress,
		Handler:      m,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
}

func readinessChecks(kubeconfigProvider machinecontroller.KubeconfigProvider) map[string]healthcheck.Check {
	return map[string]healthcheck.Check{
		"valid-info-kubeconfig": func() error {
			cm, err := kubeconfigProvider.GetKubeconfig()
			if err != nil {
				klog.V(2).Infof("[healthcheck] Unable to get kubeconfig: %v", err)
				return err
			}
			if len(cm.Clusters) != 1 {
				err := errors.New("[healthcheck] Invalid kubeconfig: no clusters found")
				klog.V(2).Info(err)
				return err
			}
			for name, c := range cm.Clusters {
				if len(c.CertificateAuthorityData) == 0 {
					err := fmt.Errorf("[healthcheck] Invalid kubeconfig: no certificate authority data was specified for kuberconfig.clusters.['%s']", name)
					klog.V(2).Info(err)
					return err
				}
				if len(c.Server) == 0 {
					err := fmt.Errorf("[healthcheck] Invalid kubeconfig: no server was specified for kuberconfig.clusters.['%s']", name)
					klog.V(2).Info(err)
					return err
				}
			}
			return nil
		},
	}
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

func parseKubeletFeatureGates(s string) (map[string]bool, error) {
	featureGates := map[string]bool{}
	sFeatureGates := strings.Split(s, ",")

	for _, featureGate := range sFeatureGates {
		sFeatureGate := strings.Split(featureGate, "=")
		if len(sFeatureGate) != 2 {
			return nil, fmt.Errorf("invalid kubelet feature gate: %q", featureGate)
		}
		featureGateEnabled, err := strconv.ParseBool(sFeatureGate[1])
		if err != nil {
			return nil, fmt.Errorf("failed to parse kubelet feature gate: %q", featureGate)
		}

		featureGates[sFeatureGate[0]] = featureGateEnabled
	}
	if len(featureGates) == 0 {
		featureGates["RotateKubeletServerCertificate"] = true
	}

	return featureGates, nil
}
