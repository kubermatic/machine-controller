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
	"reflect"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"

	"github.com/docker/distribution/reference"
	"github.com/golang/glog"
	"github.com/heptiolabs/healthcheck"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1/migrations"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/clusterinfo"
	machinecontroller "github.com/kubermatic/machine-controller/pkg/controller/machine"
	machinehealth "github.com/kubermatic/machine-controller/pkg/health"
	machinesv1alpha1 "github.com/kubermatic/machine-controller/pkg/machines/v1alpha1"
	"github.com/kubermatic/machine-controller/pkg/signals"

	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	clusterv1alpha1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	clusterv1alpha1clientset "sigs.k8s.io/cluster-api/pkg/client/clientset_generated/clientset"
	clusterinformers "sigs.k8s.io/cluster-api/pkg/client/informers_generated/externalversions"
	clusterlistersv1alpha1 "sigs.k8s.io/cluster-api/pkg/client/listers_generated/cluster/v1alpha1"
	machinedeploymentcontroller "sigs.k8s.io/cluster-api/pkg/controller/machinedeployment"
	machinesetcontroller "sigs.k8s.io/cluster-api/pkg/controller/machineset"
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

	nodeHTTPProxy          string
	nodeNoProxy            string
	nodeInsecureRegistries string
	nodeRegistryMirrors    string
	nodePauseImage         string
	nodeHyperkubeImage     string
)

const (
	controllerName                     = "machine-controller"
	defaultLeaderElectionNamespace     = "kube-system"
	defaultLeaderElectionLeaseDuration = 15 * time.Second
	defaultLeaderElectionRenewDeadline = 10 * time.Second
	defaultLeaderElectionRetryPeriod   = 2 * time.Second

	controllerNameLabelKey = "machine.k8s.io/controller"
)

// controllerRunOptions holds data that are required to create and run machine controller
type controllerRunOptions struct {
	// kubeClient a client that knows how to consume kubernetes API
	kubeClient *kubernetes.Clientset

	// extClient a client that knows how to consume kubernetes extension API
	extClient *apiextclient.Clientset

	// machineClient a client that knows how to consume Machine resources
	machineClient *clusterv1alpha1clientset.Clientset

	// ctrlruntimeclient is a client that knows how to consume everything
	ctrlruntimeClient ctrlruntimeclient.Client

	// metrics a struct that holds all metrics we want to collect
	metrics *machinecontroller.MetricsCollection

	// leaderElectionClient holds a client that is used by the leader election library
	leaderElectionClient *kubernetes.Clientset

	// nodeInformer holds a shared informer for Nodes
	nodeInformer cache.SharedIndexInformer

	// nodeLister holds a lister that knows how to list Nodes from a cache
	nodeLister listerscorev1.NodeLister

	// secretSystemNsLister knows hot to list Secrects that are inside kube-system namespace from a cache
	secretSystemNsLister listerscorev1.SecretLister

	// pvLister knows how to list PersistentVolumes
	pvLister listerscorev1.PersistentVolumeLister

	// machineInformer holds a shared informer for Machines
	machineInformer cache.SharedIndexInformer

	// machineLister holds a lister that knows how to list Machines from a cache
	machineLister clusterlistersv1alpha1.MachineLister

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

	node machinecontroller.NodeSettings
}

func main() {
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
	flag.StringVar(&nodeHyperkubeImage, "node-hyperkube-image", "k8s.gcr.io/hyperkube-amd64", "Image for the hyperkube container excluding tag.")

	flag.Parse()
	kubeconfig = flag.Lookup("kubeconfig").Value.(flag.Getter).Get().(string)
	masterURL = flag.Lookup("master").Value.(flag.Getter).Get().(string)

	clusterDNSIPs, err := parseClusterDNSIPs(clusterDNSIPs)
	if err != nil {
		glog.Fatalf("invalid cluster dns specified: %v", err)
	}

	var parsedJoinClusterTimeout *time.Duration
	if joinClusterTimeout != "" {
		parsedJoinClusterTimeoutLiteral, err := time.ParseDuration(joinClusterTimeout)
		parsedJoinClusterTimeout = &parsedJoinClusterTimeoutLiteral
		if err != nil {
			glog.Fatalf("failed to parse join-cluster-timeout as duration: %v", err)
		}
	}

	stopCh := signals.SetupSignalHandler()

	// Needed for migrations
	if err := machinesv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		glog.Fatalf("failed to add machinesv1alpha1 api to scheme: %v", err)
	}
	if err := apiextensionsv1beta1.AddToScheme(scheme.Scheme); err != nil {
		glog.Fatalf("failed to add apiextensionv1beta1 api to scheme: %v", err)
	}
	if err := clusterv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		glog.Fatalf("failed to add clusterv1alpha1 api to scheme: %v", err)
	}
	// Check if the hyperkube image has a tag set
	hyperkubeImageRef, err := reference.Parse(nodeHyperkubeImage)
	if err != nil {
		glog.Fatalf("failed to parse --node-hyperkube-image %s: %v", nodeHyperkubeImage, err)
	}
	if _, ok := hyperkubeImageRef.(reference.NamedTagged); ok {
		glog.Fatalf("--node-hyperkube-image must not contain a tag. The tag will be dynamically set for each Machine.")
	}

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		glog.Fatalf("error building kubeconfig: %v", err)
	}

	// rest.Config has no DeepCopy() that returns another rest.Config, thus
	// we simply build it twice
	// We need a dedicated one for machines because we want to increate the
	// QPS and Burst config there
	machineCfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		glog.Fatalf("error building kubeconfig for machines: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("error building kubernetes clientset for kubeClient: %v", err)
	}

	extClient, err := apiextclient.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("error building kubernetes clientset for extClient: %v", err)
	}

	ctrlruntimeClient, err := ctrlruntimeclient.New(cfg, ctrlruntimeclient.Options{})
	if err != nil {
		glog.Fatalf("error building ctrlruntime client: %v", err)
	}

	// We do a huge amount of requests when processing some more machines
	// as this controller still does defaulting and there is no separate status
	// object so conflicts happen often which results in retries
	machineCfg.QPS = 20
	machineCfg.Burst = 50
	machineClient, err := clusterv1alpha1clientset.NewForConfig(machineCfg)
	if err != nil {
		glog.Fatalf("error building example clientset for machineClient: %v", err)
	}

	leaderElectionClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("error building kubernetes clientset for leaderElectionClient: %v", err)
	}

	prometheusRegistry := prometheus.DefaultRegisterer

	// before we acquire a lock we actually warm up caches mirroring the state of the API server
	clusterInformerFactory := clusterinformers.NewFilteredSharedInformerFactory(machineClient, time.Minute*15, metav1.NamespaceAll, labelSelector(name))
	kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Minute*15)
	kubePublicKubeInformerFactory := kubeinformers.NewFilteredSharedInformerFactory(kubeClient, time.Second*30, metav1.NamespacePublic, nil)
	kubeSystemInformerFactory := kubeinformers.NewFilteredSharedInformerFactory(kubeClient, time.Second*30, metav1.NamespaceSystem, nil)
	defaultKubeInformerFactory := kubeinformers.NewFilteredSharedInformerFactory(kubeClient, time.Second*30, metav1.NamespaceDefault, nil)

	kubeconfigProvider := clusterinfo.New(cfg, kubePublicKubeInformerFactory.Core().V1().ConfigMaps().Lister(), defaultKubeInformerFactory.Core().V1().Endpoints().Lister())
	runOptions := controllerRunOptions{
		kubeClient:        kubeClient,
		extClient:         extClient,
		machineClient:     machineClient,
		ctrlruntimeClient: ctrlruntimeClient,
		metrics:           machinecontroller.NewMachineControllerMetrics(),

		leaderElectionClient:  leaderElectionClient,
		nodeInformer:          kubeInformerFactory.Core().V1().Nodes().Informer(),
		nodeLister:            kubeInformerFactory.Core().V1().Nodes().Lister(),
		secretSystemNsLister:  kubeSystemInformerFactory.Core().V1().Secrets().Lister(),
		pvLister:              kubeInformerFactory.Core().V1().PersistentVolumes().Lister(),
		machineInformer:       clusterInformerFactory.Cluster().V1alpha1().Machines().Informer(),
		machineLister:         clusterInformerFactory.Cluster().V1alpha1().Machines().Lister(),
		kubeconfigProvider:    kubeconfigProvider,
		name:                  name,
		prometheusRegisterer:  prometheusRegistry,
		cfg:                   machineCfg,
		externalCloudProvider: externalCloudProvider,
		skipEvictionAfter:     skipEvictionAfter,
		node: machinecontroller.NodeSettings{
			ClusterDNSIPs:  clusterDNSIPs,
			HTTPProxy:      nodeHTTPProxy,
			NoProxy:        nodeNoProxy,
			HyperkubeImage: nodeHyperkubeImage,
			PauseImage:     nodePauseImage,
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
			glog.Fatalf("Splitting the bootstrap-token-service-account-name flag value in '/' returned %d parts, expected exactly two", flagPartsLen)
		}
		runOptions.bootstrapTokenServiceAccountName = &types.NamespacedName{Namespace: flagParts[0], Name: flagParts[1]}
	}

	kubeInformerFactory.Start(stopCh)
	kubePublicKubeInformerFactory.Start(stopCh)
	defaultKubeInformerFactory.Start(stopCh)
	clusterInformerFactory.Start(stopCh)
	kubeSystemInformerFactory.Start(stopCh)

	syncsMaps := []map[reflect.Type]bool{
		kubeInformerFactory.WaitForCacheSync(stopCh),
		kubePublicKubeInformerFactory.WaitForCacheSync(stopCh),
		clusterInformerFactory.WaitForCacheSync(stopCh),
		defaultKubeInformerFactory.WaitForCacheSync(stopCh),
		kubeSystemInformerFactory.WaitForCacheSync(stopCh),
	}
	for _, syncsMap := range syncsMaps {
		for key, synced := range syncsMap {
			if !synced {
				glog.Fatalf("unable to sync %s", key)
			}
		}
	}

	ctx, ctxDone := context.WithCancel(context.Background())
	var g run.Group
	{
		prometheusRegistry.MustRegister(machinecontroller.NewMachineCollector(
			clusterInformerFactory.Cluster().V1alpha1().Machines().Lister(),
			kubeClient,
		))

		s := createUtilHTTPServer(kubeClient, kubeconfigProvider, prometheus.DefaultGatherer)
		g.Add(func() error {
			return s.ListenAndServe()
		}, func(err error) {
			glog.Warningf("shutting down HTTP server due to: %s", err)
			srvCtx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			if err = s.Shutdown(srvCtx); err != nil {
				glog.Errorf("failed to shutdown HTTP server: %s", err)
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

	glog.Info(g.Run())
}

// startControllerViaLeaderElection starts machine controller only if a proper lock was acquired.
// This essentially means that we can have multiple instances and at the same time only one is operational.
// The program terminates when the leadership was lost.
func startControllerViaLeaderElection(runOptions controllerRunOptions) error {
	id, err := os.Hostname()
	if err != nil {
		glog.Fatalf("error getting hostname: %s", err.Error())
	}
	// add a seed to the id, so that two processes on the same host don't accidentally both become active
	id = id + "_" + string(uuid.NewUUID())

	// add worker name to the election lock name to prevent conflicts between controllers handling different worker labels
	leaderName := controllerName
	if runOptions.name != "" {
		leaderName = runOptions.name + "-" + leaderName
	}

	rl := resourcelock.EndpointsLock{
		EndpointsMeta: metav1.ObjectMeta{
			Namespace: defaultLeaderElectionNamespace,
			Name:      leaderName,
		},
		Client: runOptions.leaderElectionClient.CoreV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity:      id + fmt.Sprintf("-%s", leaderName),
			EventRecorder: createRecorder(runOptions.kubeClient),
		},
	}

	// I think this might be a bit paranoid but the fact the there is no way
	// to stop the leader election library might cause synchronization issues.
	// imagine that a user wants to shutdown the app but since there is no way of telling the library to stop it will eventually run `runController` method
	// and bad things can happen - the fact it works at the moment doesn't mean it will in the future
	runController := func(ctx context.Context) {

		mgrSyncPeriod := 5 * time.Minute
		mgr, err := manager.New(runOptions.cfg, manager.Options{SyncPeriod: &mgrSyncPeriod})
		if err != nil {
			glog.Errorf("failed to create manager: %v", err)
			runOptions.parentCtxDone()
			return
		}

		providerData := &cloudprovidertypes.ProviderData{
			PVLister: runOptions.pvLister,
			Update:   cloudprovidertypes.GetMachineUpdater(ctx, mgr.GetClient()),
		}

		//Migrate MachinesV1Alpha1Machine to ClusterV1Alpha1Machine
		if err := migrations.MigrateMachinesv1Alpha1MachineToClusterv1Alpha1MachineIfNecessary(ctx, runOptions.ctrlruntimeClient, runOptions.kubeClient, providerData); err != nil {
			glog.Errorf("Migration to clusterv1alpha1 failed: %v", err)
			runOptions.parentCtxDone()
			return
		}

		//Migrate providerConfig field to providerSpec field
		if err := migrations.MigrateProviderConfigToProviderSpecIfNecesary(ctx, runOptions.cfg, runOptions.ctrlruntimeClient); err != nil {
			glog.Errorf("Migration of providerConfig field to providerSpec field failed: %v", err)
			runOptions.parentCtxDone()
			return
		}

		if err := machinesetcontroller.Add(mgr); err != nil {
			glog.Errorf("failed to add MachineSet controller to manager: %v", err)
			runOptions.parentCtxDone()
			return
		}
		if err := machinedeploymentcontroller.Add(mgr); err != nil {
			glog.Errorf("failed to add MachineDeployment controller to manager: %v", err)
			runOptions.parentCtxDone()
			return
		}
		go func() {
			if err := mgr.Start(runOptions.parentCtx.Done()); err != nil {
				glog.Errorf("failed to start kubebuilder manager: %v", err)
				runOptions.parentCtxDone()
				return
			}
		}()

		machineController, err := machinecontroller.NewMachineController(
			runOptions.kubeClient,
			mgr.GetClient(),
			runOptions.metrics,
			runOptions.prometheusRegisterer,
			runOptions.machineInformer,
			runOptions.nodeInformer,
			runOptions.kubeconfigProvider,
			providerData,
			runOptions.joinClusterTimeout,
			runOptions.externalCloudProvider,
			runOptions.name,
			runOptions.bootstrapTokenServiceAccountName,
			runOptions.skipEvictionAfter,
			runOptions.node,
		)
		if err != nil {
			glog.Errorf("failed to create machine-controller: %v", err)
			runOptions.parentCtxDone()
			return
		}

		if runErr := machineController.Run(workerCount, runOptions.parentCtx.Done()); runErr != nil {
			glog.Errorf("error running controller: %v", runErr)
			runOptions.parentCtxDone()
			return
		}
		glog.Info("machine controller has been successfully stopped")
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
				glog.V(2).Infof("[healthcheck] Unable to get kubeconfig: %v", err)
				return err
			}
			if len(cm.Clusters) != 1 {
				err := errors.New("[healthcheck] Invalid kubeconfig: no clusters found")
				glog.V(2).Info(err)
				return err
			}
			for name, c := range cm.Clusters {
				if len(c.CertificateAuthorityData) == 0 {
					err := fmt.Errorf("[healthcheck] Invalid kubeconfig: no certificate authority data was specified for kuberconfig.clusters.['%s']", name)
					glog.V(2).Info(err)
					return err
				}
				if len(c.Server) == 0 {
					err := fmt.Errorf("[healthcheck] Invalid kubeconfig: no server was specified for kuberconfig.clusters.['%s']", name)
					glog.V(2).Info(err)
					return err
				}
			}
			return nil
		},
	}
}

// createRecorder creates a new event recorder which is later used by the leader election
// library to broadcast events
func createRecorder(kubeClient *kubernetes.Clientset) record.EventRecorder {
	glog.V(3).Info("creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.V(3).Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	return eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerName})
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

// return label selector to only process machines with a matching machine.k8s.io/controller label
func labelSelector(workerName string) func(*metav1.ListOptions) {
	return func(options *metav1.ListOptions) {
		var req *labels.Requirement
		var err error
		if workerName == "" {
			if req, err = labels.NewRequirement(controllerNameLabelKey, selection.DoesNotExist, nil); err != nil {
				glog.Fatalf("failed to build label selector: %v", err)
			}
		} else {
			if req, err = labels.NewRequirement(controllerNameLabelKey, selection.Equals, []string{workerName}); err != nil {
				glog.Fatalf("failed to build label selector: %v", err)
			}
		}

		options.LabelSelector = req.String()
	}
}
