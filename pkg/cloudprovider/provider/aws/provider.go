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

package aws

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/sts"
	gocache "github.com/patrickmn/go-cache"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/kubermatic/machine-controller/pkg/apis/cluster/common"
	clusterv1alpha1 "github.com/kubermatic/machine-controller/pkg/apis/cluster/v1alpha1"
	cloudprovidererrors "github.com/kubermatic/machine-controller/pkg/cloudprovider/errors"
	"github.com/kubermatic/machine-controller/pkg/cloudprovider/instance"
	awstypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/provider/aws/types"
	cloudprovidertypes "github.com/kubermatic/machine-controller/pkg/cloudprovider/types"
	"github.com/kubermatic/machine-controller/pkg/providerconfig"
	providerconfigtypes "github.com/kubermatic/machine-controller/pkg/providerconfig/types"
	"github.com/kubermatic/machine-controller/pkg/userdata/convert"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	// Interval and timeout for polling.
	pollInterval = 2 * time.Second
	pollTimeout  = 5 * time.Minute
)

var (
	metricInstancesForMachines = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "machine_controller_aws_instances_for_machine",
		Help: "The number of instances at aws for a given machine"}, []string{"machine"})
)

func init() {
	metrics.Registry.MustRegister(metricInstancesForMachines)
}

type provider struct {
	configVarResolver *providerconfig.ConfigVarResolver
}

// New returns a aws provider
func New(configVarResolver *providerconfig.ConfigVarResolver) cloudprovidertypes.Provider {
	return &provider{configVarResolver: configVarResolver}
}

const (
	nameTag       = "Name"
	machineUIDTag = "Machine-UID"

	maxRetries = 100
)

var (
	volumeTypes = sets.NewString(
		ec2.VolumeTypeStandard,
		ec2.VolumeTypeIo1,
		ec2.VolumeTypeGp2,
		ec2.VolumeTypeGp3,
		ec2.VolumeTypeSc1,
		ec2.VolumeTypeSt1,
	)

	amiFilters = map[providerconfigtypes.OperatingSystem]map[awstypes.CPUArchitecture]amiFilter{
		// Source: https://wiki.centos.org/Cloud/AWS
		providerconfigtypes.OperatingSystemCentOS: {
			awstypes.CPUArchitectureX86_64: {
				description: "CentOS 7* x86_64",
				// The AWS marketplace ID from CentOS Community Platform Engineering (CPE)
				owner: "125523088429",
			},
			awstypes.CPUArchitectureARM64: {
				description: "CentOS 7* aarch64",
				// The AWS marketplace ID from CentOS Community Platform Engineering (CPE)
				owner: "125523088429",
			},
		},
		providerconfigtypes.OperatingSystemAmazonLinux2: {
			awstypes.CPUArchitectureX86_64: {
				description: "Amazon Linux 2 AMI * x86_64 HVM gp2",
				// The AWS marketplace ID from Amazon
				owner: "137112412989",
			},
			awstypes.CPUArchitectureARM64: {
				description: "Amazon Linux 2 LTS Arm64 AMI * arm64 HVM gp2",
				// The AWS marketplace ID from Amazon
				owner: "137112412989",
			},
		},
		providerconfigtypes.OperatingSystemUbuntu: {
			awstypes.CPUArchitectureX86_64: {
				// Be as precise as possible - otherwise we might get a nightly dev build
				description: "Canonical, Ubuntu, 20.04 LTS, amd64 focal image build on ????-??-??",
				// The AWS marketplace ID from Canonical
				owner: "099720109477",
			},
			awstypes.CPUArchitectureARM64: {
				// Be as precise as possible - otherwise we might get a nightly dev build
				description: "Canonical, Ubuntu, 20.04 LTS, arm64 focal image build on ????-??-??",
				// The AWS marketplace ID from Canonical
				owner: "099720109477",
			},
		},
		providerconfigtypes.OperatingSystemSLES: {
			awstypes.CPUArchitectureX86_64: {
				// Be as precise as possible - otherwise we might get a nightly dev build
				description: "SUSE Linux Enterprise Server 15 SP1 (HVM, 64-bit, SSD-Backed)",
				// The AWS marketplace ID from SLES
				owner: "013907871322",
			},
			awstypes.CPUArchitectureARM64: {
				// Be as precise as possible - otherwise we might get a nightly dev build
				description: "SUSE Linux Enterprise Server 15 SP1 (HVM, 64-bit, SSD-Backed)",
				// The AWS marketplace ID from SLES
				owner: "013907871322",
			},
		},
		providerconfigtypes.OperatingSystemRHEL: {
			awstypes.CPUArchitectureX86_64: {
				// Be as precise as possible - otherwise we might get a nightly dev build
				description: "Provided by Red Hat, Inc.",
				// The AWS marketplace ID from RedHat
				owner: "309956199498",
			},
			awstypes.CPUArchitectureARM64: {
				// Be as precise as possible - otherwise we might get a nightly dev build
				description: "Provided by Red Hat, Inc.",
				// The AWS marketplace ID from RedHat
				owner: "309956199498",
			},
		},
		providerconfigtypes.OperatingSystemFlatcar: {
			awstypes.CPUArchitectureX86_64: {
				// Be as precise as possible - otherwise we might get a nightly dev build
				description: "Flatcar Container Linux stable *",
				// The AWS marketplace ID from AWS
				owner: "075585003325",
			},
			// 2021-10-14 - Flatcar stable does not support ARM yet (only alpha channels supports it)
		},
	}

	// cacheLock protects concurrent cache misses against a single key. This usually happens when multiple machines get created simultaneously
	// We lock so the first access updates/writes the data to the cache and afterwards everyone reads the cached data
	cacheLock = &sync.Mutex{}
	cache     = gocache.New(5*time.Minute, 5*time.Minute)
)

type Config struct {
	AccessKeyID     string
	SecretAccessKey string

	Region             string
	AvailabilityZone   string
	VpcID              string
	SubnetID           string
	SecurityGroupIDs   []string
	InstanceProfile    string
	InstanceType       string
	AMI                string
	DiskSize           int64
	DiskType           string
	DiskIops           *int64
	EBSVolumeEncrypted bool
	Tags               map[string]string
	AssignPublicIP     *bool

	IsSpotInstance           *bool
	SpotMaxPrice             *string
	SpotPersistentRequest    *bool
	SpotInterruptionBehavior *string

	AssumeRoleARN        string
	AssumeRoleExternalID string
}

type amiFilter struct {
	description string
	owner       string
	productCode string
}

func getDefaultAMIID(client *ec2.EC2, os providerconfigtypes.OperatingSystem, region string, cpuArchitecture awstypes.CPUArchitecture) (string, error) {
	cacheLock.Lock()
	defer cacheLock.Unlock()

	osFilter, osSupported := amiFilters[os]
	if !osSupported {
		return "", fmt.Errorf("operating system %q not supported", os)
	}

	filter, archSupported := osFilter[cpuArchitecture]
	if !archSupported {
		return "", fmt.Errorf("CPU architecture '%s' not supported for operating system '%s'", cpuArchitecture, os)
	}

	cacheKey := fmt.Sprintf("ami-id-%s-%s-%s", region, os, cpuArchitecture)
	amiID, found := cache.Get(cacheKey)
	if found {
		klog.V(3).Info("found AMI-ID in cache!")
		return amiID.(string), nil
	}

	describeImagesInput := &ec2.DescribeImagesInput{
		Owners: aws.StringSlice([]string{filter.owner}),
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("description"),
				Values: aws.StringSlice([]string{filter.description}),
			},
			{
				Name:   aws.String("virtualization-type"),
				Values: aws.StringSlice([]string{"hvm"}),
			},
			{
				Name:   aws.String("root-device-type"),
				Values: aws.StringSlice([]string{"ebs"}),
			},
			{
				Name:   aws.String("architecture"),
				Values: aws.StringSlice([]string{string(cpuArchitecture)}),
			},
		},
	}

	if filter.productCode != "" {
		describeImagesInput.Filters = append(describeImagesInput.Filters, &ec2.Filter{
			Name:   aws.String("product-code"),
			Values: aws.StringSlice([]string{filter.productCode}),
		})
	}

	imagesOut, err := client.DescribeImages(describeImagesInput)
	if err != nil {
		return "", err
	}

	if len(imagesOut.Images) == 0 {
		return "", fmt.Errorf("could not find Image for '%s' with arch '%s'", os, cpuArchitecture)
	}

	if os == providerconfigtypes.OperatingSystemRHEL {
		imagesOut.Images, err = filterSupportedRHELImages(imagesOut.Images)
		if err != nil {
			return "", err
		}
	}

	image := imagesOut.Images[0]
	for _, v := range imagesOut.Images {
		itime, _ := time.Parse(time.RFC3339, *image.CreationDate)
		vtime, _ := time.Parse(time.RFC3339, *v.CreationDate)
		if vtime.After(itime) {
			image = v
		}
	}

	cache.SetDefault(cacheKey, *image.ImageId)
	return *image.ImageId, nil
}

func getCPUArchitecture(client *ec2.EC2, instanceType string) (awstypes.CPUArchitecture, error) {
	// read the instance type to know which cpu architecture is needed in the AMI
	instanceTypes, err := client.DescribeInstanceTypes(&ec2.DescribeInstanceTypesInput{
		InstanceTypes: []*string{aws.String(instanceType)},
	})

	if err != nil {
		return "", err
	}

	if len(instanceTypes.InstanceTypes) != 1 {
		return "", fmt.Errorf("unexpected length of instance type list: %d", len(instanceTypes.InstanceTypes))
	}

	if instanceTypes.InstanceTypes[0].ProcessorInfo != nil &&
		len(instanceTypes.InstanceTypes[0].ProcessorInfo.SupportedArchitectures) > 0 {
		for _, v := range instanceTypes.InstanceTypes[0].ProcessorInfo.SupportedArchitectures {
			// machine-controller currently supports x86_64 and ARM64, so only CPU architectures
			// that are supported will be returned if found in the AWS API response
			if arch := awstypes.CPUArchitecture(*v); arch == awstypes.CPUArchitectureX86_64 || arch == awstypes.CPUArchitectureARM64 {
				return arch, nil
			}
		}
	}

	return "", errors.New("returned instance type data did not include supported architectures")
}

func getDefaultRootDevicePath(os providerconfigtypes.OperatingSystem) (string, error) {
	const (
		rootDevicePathSDA  = "/dev/sda1"
		rootDevicePathXVDA = "/dev/xvda"
	)

	switch os {
	case providerconfigtypes.OperatingSystemUbuntu:
		return rootDevicePathSDA, nil
	case providerconfigtypes.OperatingSystemCentOS:
		return rootDevicePathSDA, nil
	case providerconfigtypes.OperatingSystemSLES:
		return rootDevicePathXVDA, nil
	case providerconfigtypes.OperatingSystemRHEL:
		return rootDevicePathSDA, nil
	case providerconfigtypes.OperatingSystemFlatcar:
		return rootDevicePathXVDA, nil
	case providerconfigtypes.OperatingSystemAmazonLinux2:
		return rootDevicePathXVDA, nil
	}

	return "", fmt.Errorf("no default root path found for %s operating system", os)
}

func (p *provider) getConfig(provSpec clusterv1alpha1.ProviderSpec) (*Config, *providerconfigtypes.Config, *awstypes.RawConfig, error) {
	if provSpec.Value == nil {
		return nil, nil, nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}

	pconfig, err := providerconfigtypes.GetConfig(provSpec)
	if err != nil {
		return nil, nil, nil, err
	}

	if pconfig.OperatingSystemSpec.Raw == nil {
		return nil, nil, nil, errors.New("operatingSystemSpec in the MachineDeployment cannot be empty")
	}

	rawConfig, err := awstypes.GetConfig(*pconfig)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to unmarshal: %v", err)
	}

	c := Config{}
	c.AccessKeyID, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.AccessKeyID, "AWS_ACCESS_KEY_ID")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"accessKeyId\" field, error = %v", err)
	}
	c.SecretAccessKey, err = p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.SecretAccessKey, "AWS_SECRET_ACCESS_KEY")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get the value of \"secretAccessKey\" field, error = %v", err)
	}
	c.Region, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.Region)
	if err != nil {
		return nil, nil, nil, err
	}
	c.VpcID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.VpcID)
	if err != nil {
		return nil, nil, nil, err
	}
	c.SubnetID, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.SubnetID)
	if err != nil {
		return nil, nil, nil, err
	}
	c.AvailabilityZone, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.AvailabilityZone)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, securityGroupIDRaw := range rawConfig.SecurityGroupIDs {
		securityGroupID, err := p.configVarResolver.GetConfigVarStringValue(securityGroupIDRaw)
		if err != nil {
			return nil, nil, nil, err
		}
		c.SecurityGroupIDs = append(c.SecurityGroupIDs, securityGroupID)
	}
	c.InstanceProfile, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.InstanceProfile)
	if err != nil {
		return nil, nil, nil, err
	}
	c.InstanceType, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.InstanceType)
	if err != nil {
		return nil, nil, nil, err
	}
	c.AMI, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.AMI)
	if err != nil {
		return nil, nil, nil, err
	}
	c.DiskSize = rawConfig.DiskSize
	c.DiskType, err = p.configVarResolver.GetConfigVarStringValue(rawConfig.DiskType)
	if err != nil {
		return nil, nil, nil, err
	}
	if c.DiskType == ec2.VolumeTypeIo1 {
		if rawConfig.DiskIops == nil {
			return nil, nil, nil, errors.New("Missing required field `diskIops`")
		}
		iops := *rawConfig.DiskIops

		if iops < 100 || iops > 64000 {
			return nil, nil, nil, errors.New("Invalid value for `diskIops` (min: 100, max: 64000)")
		}

		c.DiskIops = rawConfig.DiskIops
	} else if c.DiskType == ec2.VolumeTypeGp3 && rawConfig.DiskIops != nil {
		// gp3 disks start with 3000 IOPS by default, we _can_ pass better IOPS, but it is not a required field
		iops := *rawConfig.DiskIops

		if iops < 3000 || iops > 64000 {
			return nil, nil, nil, errors.New("Invalid value for `diskIops` (min: 3000, max: 64000)")
		}

		c.DiskIops = rawConfig.DiskIops
	}

	c.EBSVolumeEncrypted, _, err = p.configVarResolver.GetConfigVarBoolValue(rawConfig.EBSVolumeEncrypted)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get ebsVolumeEncrypted value: %v", err)
	}
	c.Tags = rawConfig.Tags
	c.AssignPublicIP = rawConfig.AssignPublicIP
	c.IsSpotInstance = rawConfig.IsSpotInstance
	if rawConfig.SpotInstanceConfig != nil && c.IsSpotInstance != nil && *c.IsSpotInstance {
		maxPrice, err := p.configVarResolver.GetConfigVarStringValue(rawConfig.SpotInstanceConfig.MaxPrice)
		if err != nil {
			return nil, nil, nil, err
		}
		c.SpotMaxPrice = pointer.StringPtr(maxPrice)

		persistentRequest, _, err := p.configVarResolver.GetConfigVarBoolValue(rawConfig.SpotInstanceConfig.PersistentRequest)
		if err != nil {
			return nil, nil, nil, err
		}
		c.SpotPersistentRequest = pointer.BoolPtr(persistentRequest)

		interruptionBehavior, err := p.configVarResolver.GetConfigVarStringValue(rawConfig.SpotInstanceConfig.InterruptionBehavior)
		if err != nil {
			return nil, nil, nil, err
		}
		c.SpotInterruptionBehavior = pointer.StringPtr(interruptionBehavior)
	}
	assumeRoleARN, err := p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.AssumeRoleARN, "AWS_ASSUME_ROLE_ARN")
	if err != nil {
		return nil, nil, nil, err
	}
	c.AssumeRoleARN = assumeRoleARN
	assumeRoleExternalID, err := p.configVarResolver.GetConfigVarStringValueOrEnv(rawConfig.AssumeRoleExternalID, "AWS_ASSUME_ROLE_EXTERNAL_ID")
	if err != nil {
		return nil, nil, nil, err
	}
	c.AssumeRoleExternalID = assumeRoleExternalID

	return &c, pconfig, rawConfig, err
}

func getSession(id, secret, token, region, assumeRoleARN, assumeRoleExternalID string) (*session.Session, error) {
	config := aws.NewConfig()
	config = config.WithRegion(region)
	config = config.WithCredentials(credentials.NewStaticCredentials(id, secret, token))
	config = config.WithMaxRetries(maxRetries)
	awsSession, err := session.NewSession(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %v", err)
	}

	// Assume IAM role of e.g. external AWS account if configured
	if assumeRoleARN != "" {
		awsSession, err = getAssumeRoleSession(awsSession, assumeRoleARN, assumeRoleExternalID, region)
		if err != nil {
			return nil, fmt.Errorf("failed to create temporary AWS session for assumed role: %v", err)
		}
	}

	return awsSession, err
}

func getAssumeRoleSession(awsSession *session.Session, assumeRoleARN, assumeRoleExternalID, region string) (*session.Session, error) {
	assumeRoleOutput, err := getAssumeRoleCredentials(awsSession, assumeRoleARN, assumeRoleExternalID)
	if err != nil {
		return nil, awsErrorToTerminalError(err, "unable to initialize aws external id session")
	}

	assumedRoleConfig := aws.NewConfig()
	assumedRoleConfig = assumedRoleConfig.WithRegion(region)
	assumedRoleConfig = assumedRoleConfig.WithCredentials(credentials.NewStaticCredentials(*assumeRoleOutput.Credentials.AccessKeyId,
		*assumeRoleOutput.Credentials.SecretAccessKey,
		*assumeRoleOutput.Credentials.SessionToken))
	assumedRoleConfig = assumedRoleConfig.WithMaxRetries(maxRetries)
	return session.NewSession(assumedRoleConfig)
}

func getAssumeRoleCredentials(session *session.Session, assumeRoleARN, assumeRoleExternalID string) (*sts.AssumeRoleOutput, error) {
	stsSession := sts.New(session)
	sessionName := "kubermatic-machine-controller"
	return stsSession.AssumeRole(&sts.AssumeRoleInput{
		ExternalId:      &assumeRoleExternalID,
		RoleArn:         &assumeRoleARN,
		RoleSessionName: &sessionName,
	})
}

func getIAMclient(id, secret, region, assumeRoleArn, assumeRoleExternalID string) (*iam.IAM, error) {
	sess, err := getSession(id, secret, "", region, assumeRoleArn, assumeRoleExternalID)
	if err != nil {
		return nil, awsErrorToTerminalError(err, "failed to get aws session")
	}
	return iam.New(sess), nil
}

func getEC2client(id, secret, region, assumeRoleArn, assumeRoleExternalID string) (*ec2.EC2, error) {
	sess, err := getSession(id, secret, "", region, assumeRoleArn, assumeRoleExternalID)
	if err != nil {
		return nil, awsErrorToTerminalError(err, "failed to get aws session")
	}
	return ec2.New(sess), nil
}

func (p *provider) AddDefaults(spec clusterv1alpha1.MachineSpec) (clusterv1alpha1.MachineSpec, error) {
	_, _, rawConfig, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return spec, err
	}
	if rawConfig.DiskType.Value == "" {
		rawConfig.DiskType.Value = ec2.VolumeTypeStandard
	}
	if rawConfig.AssignPublicIP == nil {
		rawConfig.AssignPublicIP = aws.Bool(true)
	}
	if rawConfig.IsSpotInstance != nil && *rawConfig.IsSpotInstance {
		if spec.Labels == nil {
			spec.Labels = map[string]string{}
		}
		spec.Labels["k8c.io/aws-spot"] = "aws-node-termination-handler"
	}
	spec.ProviderSpec.Value, err = setProviderSpec(*rawConfig, spec.ProviderSpec)
	return spec, err
}

func (p *provider) Validate(spec clusterv1alpha1.MachineSpec) error {
	config, pc, _, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	if _, osSupported := amiFilters[pc.OperatingSystem]; !osSupported {
		return fmt.Errorf("unsupported os %s", pc.OperatingSystem)
	}

	if !volumeTypes.Has(config.DiskType) {
		return fmt.Errorf("invalid volume type %s specified. Supported: %s", config.DiskType, volumeTypes)
	}

	if config.InstanceType == "" {
		return fmt.Errorf("instanceType must be specified")
	}

	// Not the best test as the minimum disk size depends on the AMI
	// but the best we can do here
	if config.DiskSize == 0 {
		return fmt.Errorf("diskSize must be specified and > 0")
	}

	ec2Client, err := getEC2client(config.AccessKeyID, config.SecretAccessKey, config.Region, config.AssumeRoleARN, config.AssumeRoleExternalID)
	if err != nil {
		return fmt.Errorf("failed to create ec2 client: %v", err)
	}
	if config.AMI != "" {
		_, err := ec2Client.DescribeImages(&ec2.DescribeImagesInput{
			ImageIds: aws.StringSlice([]string{config.AMI}),
		})
		if err != nil {
			return fmt.Errorf("failed to validate ami: %v", err)
		}
	}

	if _, err := getVpc(ec2Client, config.VpcID); err != nil {
		return fmt.Errorf("invalid vpc %q specified: %v", config.VpcID, err)
	}

	_, err = ec2Client.DescribeAvailabilityZones(&ec2.DescribeAvailabilityZonesInput{ZoneNames: aws.StringSlice([]string{config.AvailabilityZone})})
	if err != nil {
		return fmt.Errorf("invalid zone %q specified: %v", config.AvailabilityZone, err)
	}

	_, err = ec2Client.DescribeRegions(&ec2.DescribeRegionsInput{RegionNames: aws.StringSlice([]string{config.Region})})
	if err != nil {
		return fmt.Errorf("invalid region %q specified: %v", config.Region, err)
	}

	if len(config.SecurityGroupIDs) == 0 {
		return errors.New("no security groups were specified")
	}
	_, err = ec2Client.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
		GroupIds: aws.StringSlice(config.SecurityGroupIDs),
	})
	if err != nil {
		return fmt.Errorf("failed to validate security group id's: %v", err)
	}

	iamClient, err := getIAMclient(config.AccessKeyID, config.SecretAccessKey, config.Region, config.AssumeRoleARN, config.AssumeRoleExternalID)
	if err != nil {
		return fmt.Errorf("failed to create iam client: %v", err)
	}

	if config.InstanceProfile == "" {
		return fmt.Errorf("invalid instance profile specified %q: %v", config.InstanceProfile, err)
	}
	if _, err := iamClient.GetInstanceProfile(&iam.GetInstanceProfileInput{InstanceProfileName: aws.String(config.InstanceProfile)}); err != nil {
		return fmt.Errorf("failed to validate instance profile: %v", err)
	}

	if config.IsSpotInstance != nil && *config.IsSpotInstance {
		if config.SpotMaxPrice == nil {
			return errors.New("failed to validate max price for the spot instance: max price cannot be empty when spot instance ")
		}
	}

	return nil
}

func getVpc(client *ec2.EC2, id string) (*ec2.Vpc, error) {
	vpcOut, err := client.DescribeVpcs(&ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			{Name: aws.String("vpc-id"), Values: []*string{aws.String(id)}},
		},
	})

	if err != nil {
		return nil, awsErrorToTerminalError(err, "failed to list vpc's")
	}

	if len(vpcOut.Vpcs) != 1 {
		return nil, fmt.Errorf("unable to find specified vpc with id %q", id)
	}

	return vpcOut.Vpcs[0], nil
}

func (p *provider) Create(machine *clusterv1alpha1.Machine, data *cloudprovidertypes.ProviderData, userdata string) (instance.Instance, error) {
	config, pc, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	ec2Client, err := getEC2client(config.AccessKeyID, config.SecretAccessKey, config.Region, config.AssumeRoleARN, config.AssumeRoleExternalID)
	if err != nil {
		return nil, err
	}

	rootDevicePath, err := getDefaultRootDevicePath(pc.OperatingSystem)
	if err != nil {
		return nil, err
	}

	amiID := config.AMI
	if amiID == "" {
		// read the instance type to know which cpu architecture is needed in the AMI
		cpuArchitecture, err := getCPUArchitecture(ec2Client, config.InstanceType)

		if err != nil {
			return nil, cloudprovidererrors.TerminalError{
				Reason:  common.InvalidConfigurationMachineError,
				Message: fmt.Sprintf("Failed to find instance type %s in region %s: %v", config.InstanceType, config.Region, err),
			}
		}

		if amiID, err = getDefaultAMIID(ec2Client, pc.OperatingSystem, config.Region, cpuArchitecture); err != nil {
			return nil, cloudprovidererrors.TerminalError{
				Reason:  common.InvalidConfigurationMachineError,
				Message: fmt.Sprintf("Failed to get AMI-ID for operating system %s in region %s: %v", pc.OperatingSystem, config.Region, err),
			}
		}
	}

	if pc.OperatingSystem != providerconfigtypes.OperatingSystemFlatcar {
		// Gzip the userdata in case we don't use Flatcar
		userdata, err = convert.GzipString(userdata)
		if err != nil {
			return nil, fmt.Errorf("failed to gzip the userdata")
		}
	}

	tags := []*ec2.Tag{
		{
			Key:   aws.String(nameTag),
			Value: aws.String(machine.Spec.Name),
		},
		{
			Key:   aws.String(machineUIDTag),
			Value: aws.String(string(machine.UID)),
		},
	}

	for k, v := range config.Tags {
		tags = append(tags, &ec2.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	var instanceMarketOptions *ec2.InstanceMarketOptionsRequest
	if config.IsSpotInstance != nil && *config.IsSpotInstance {
		spotOpts := &ec2.SpotMarketOptions{
			SpotInstanceType: pointer.StringPtr(ec2.SpotInstanceTypeOneTime),
		}

		if config.SpotMaxPrice != nil && *config.SpotMaxPrice != "" {
			spotOpts.MaxPrice = config.SpotMaxPrice
		}

		if config.SpotPersistentRequest != nil && *config.SpotPersistentRequest {
			spotOpts.SpotInstanceType = pointer.StringPtr(ec2.SpotInstanceTypePersistent)
			spotOpts.InstanceInterruptionBehavior = pointer.StringPtr(ec2.InstanceInterruptionBehaviorStop)

			if config.SpotInterruptionBehavior != nil && *config.SpotInterruptionBehavior != "" {
				spotOpts.InstanceInterruptionBehavior = config.SpotInterruptionBehavior
			}
		}

		instanceMarketOptions = &ec2.InstanceMarketOptionsRequest{
			MarketType:  aws.String(ec2.MarketTypeSpot),
			SpotOptions: spotOpts,
		}
	}

	// By default we assign a public IP - We introduced this field later, so we made it a pointer & default to true.
	// This must be done aside from the webhook defaulting as we might have machines which don't get defaulted before this
	assignPublicIP := config.AssignPublicIP == nil || *config.AssignPublicIP

	instanceRequest := &ec2.RunInstancesInput{
		ImageId:               aws.String(amiID),
		InstanceMarketOptions: instanceMarketOptions,
		BlockDeviceMappings: []*ec2.BlockDeviceMapping{
			{
				DeviceName: aws.String(rootDevicePath),
				Ebs: &ec2.EbsBlockDevice{
					VolumeSize:          aws.Int64(config.DiskSize),
					DeleteOnTermination: aws.Bool(true),
					VolumeType:          aws.String(config.DiskType),
					Iops:                config.DiskIops,
					Encrypted:           pointer.BoolPtr(config.EBSVolumeEncrypted),
				},
			},
		},
		MaxCount:     aws.Int64(1),
		MinCount:     aws.Int64(1),
		InstanceType: aws.String(config.InstanceType),
		UserData:     aws.String(base64.StdEncoding.EncodeToString([]byte(userdata))),
		Placement: &ec2.Placement{
			AvailabilityZone: aws.String(config.AvailabilityZone),
		},
		NetworkInterfaces: []*ec2.InstanceNetworkInterfaceSpecification{
			{
				DeviceIndex:              aws.Int64(0), // eth0
				AssociatePublicIpAddress: aws.Bool(assignPublicIP),
				DeleteOnTermination:      aws.Bool(true),
				SubnetId:                 aws.String(config.SubnetID),
				Groups:                   aws.StringSlice(config.SecurityGroupIDs),
			},
		},
		IamInstanceProfile: &ec2.IamInstanceProfileSpecification{
			Name: aws.String(config.InstanceProfile),
		},
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String(ec2.ResourceTypeInstance),
				Tags:         tags,
			},
		},
	}

	runOut, err := ec2Client.RunInstances(instanceRequest)
	if err != nil {
		return nil, awsErrorToTerminalError(err, "failed create instance at aws")
	}

	if err = p.waitForInstance(machine); err != nil {
		return nil, awsErrorToTerminalError(err, "failed provision instance at aws")
	}

	return &awsInstance{instance: runOut.Instances[0]}, nil
}

func (p *provider) Cleanup(machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (bool, error) {
	ec2instance, err := p.get(machine)
	if err != nil {
		if err == cloudprovidererrors.ErrInstanceNotFound {
			return true, nil
		}
		return false, err
	}

	//(*Config, *providerconfigtypes.Config, *awstypes.RawConfig, error)
	config, _, _, err := p.getConfig(machine.Spec.ProviderSpec)

	if err != nil {
		return false, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	ec2Client, err := getEC2client(config.AccessKeyID, config.SecretAccessKey, config.Region, config.AssumeRoleARN, config.AssumeRoleExternalID)
	if err != nil {
		return false, err
	}

	if config.IsSpotInstance != nil && *config.IsSpotInstance &&
		config.SpotPersistentRequest != nil && *config.SpotPersistentRequest {

		cOut, err := ec2Client.CancelSpotInstanceRequests(&ec2.CancelSpotInstanceRequestsInput{
			SpotInstanceRequestIds: aws.StringSlice([]string{*ec2instance.instance.SpotInstanceRequestId}),
		})

		if err != nil {
			return false, awsErrorToTerminalError(err, "failed to cancel spot instance request")
		}

		if *cOut.CancelledSpotInstanceRequests[0].State == ec2.CancelSpotInstanceRequestStateCancelled {
			klog.V(3).Infof("successfully canceled spot instance request %s at aws", *ec2instance.instance.SpotInstanceRequestId)
		}
	}

	tOut, err := ec2Client.TerminateInstances(&ec2.TerminateInstancesInput{
		InstanceIds: aws.StringSlice([]string{ec2instance.ID()}),
	})
	if err != nil {
		return false, awsErrorToTerminalError(err, "failed to terminate instance")
	}

	if *tOut.TerminatingInstances[0].PreviousState.Name != *tOut.TerminatingInstances[0].CurrentState.Name {
		klog.V(3).Infof("successfully triggered termination of instance %s at aws", ec2instance.ID())
	}

	return false, nil
}

func (p *provider) Get(machine *clusterv1alpha1.Machine, _ *cloudprovidertypes.ProviderData) (instance.Instance, error) {
	return p.get(machine)
}

func (p *provider) get(machine *clusterv1alpha1.Machine) (*awsInstance, error) {
	config, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	ec2Client, err := getEC2client(config.AccessKeyID, config.SecretAccessKey, config.Region, config.AssumeRoleARN, config.AssumeRoleExternalID)
	if err != nil {
		return nil, err
	}

	inOut, err := ec2Client.DescribeInstances(&ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("tag:" + machineUIDTag),
				Values: aws.StringSlice([]string{string(machine.UID)}),
			},
		},
	})
	if err != nil {
		return nil, awsErrorToTerminalError(err, "failed to list instances from aws")
	}

	// We might have multiple instances (Maybe some old, terminated ones)
	// Thus we need to find the instance which is not in the terminated state
	for _, reservation := range inOut.Reservations {
		for _, i := range reservation.Instances {
			if i.State == nil || i.State.Name == nil {
				continue
			}

			if *i.State.Name == ec2.InstanceStateNameTerminated {
				continue
			}

			return &awsInstance{
				instance: i,
			}, nil
		}
	}

	return nil, cloudprovidererrors.ErrInstanceNotFound
}

func (p *provider) GetCloudConfig(spec clusterv1alpha1.MachineSpec) (config string, name string, err error) {
	c, _, _, err := p.getConfig(spec.ProviderSpec)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse config: %v", err)
	}

	cc := &awstypes.CloudConfig{
		Global: awstypes.GlobalOpts{
			VPC:      c.VpcID,
			SubnetID: c.SubnetID,
			Zone:     c.AvailabilityZone,
		},
	}

	s, err := awstypes.CloudConfigToString(cc)
	if err != nil {
		return "", "", fmt.Errorf("failed to convert cloud-config to string: %v", err)
	}

	return s, "aws", nil

}

func (p *provider) MachineMetricsLabels(machine *clusterv1alpha1.Machine) (map[string]string, error) {
	labels := make(map[string]string)

	c, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err == nil {
		labels["size"] = c.InstanceType
		labels["region"] = c.Region
		labels["az"] = c.AvailabilityZone
		labels["ami"] = c.AMI
	}

	return labels, err
}

func (p *provider) MigrateUID(machine *clusterv1alpha1.Machine, new types.UID) error {
	machineInstance, err := p.get(machine)
	if err != nil {
		if err == cloudprovidererrors.ErrInstanceNotFound {
			return nil
		}
		return fmt.Errorf("failed to get instance: %v", err)
	}

	config, _, _, err := p.getConfig(machine.Spec.ProviderSpec)
	if err != nil {
		return cloudprovidererrors.TerminalError{
			Reason:  common.InvalidConfigurationMachineError,
			Message: fmt.Sprintf("Failed to parse MachineSpec, due to %v", err),
		}
	}

	ec2Client, err := getEC2client(config.AccessKeyID, config.SecretAccessKey, config.Region, config.AssumeRoleARN, config.AssumeRoleExternalID)
	if err != nil {
		return fmt.Errorf("failed to get EC2 client: %v", err)
	}

	_, err = ec2Client.CreateTags(&ec2.CreateTagsInput{
		Resources: aws.StringSlice([]string{machineInstance.ID()}),
		Tags:      []*ec2.Tag{{Key: aws.String(machineUIDTag), Value: aws.String(string(new))}}})
	if err != nil {
		return fmt.Errorf("failed to update instance with new machineUIDTag: %v", err)
	}

	return nil
}

type awsInstance struct {
	instance *ec2.Instance
}

func (d *awsInstance) Name() string {
	return getTagValue(nameTag, d.instance.Tags)
}

func (d *awsInstance) ID() string {
	return aws.StringValue(d.instance.InstanceId)
}

func (d *awsInstance) Addresses() map[string]v1.NodeAddressType {
	addresses := map[string]v1.NodeAddressType{
		aws.StringValue(d.instance.PublicIpAddress):  v1.NodeExternalIP,
		aws.StringValue(d.instance.PublicDnsName):    v1.NodeExternalDNS,
		aws.StringValue(d.instance.PrivateIpAddress): v1.NodeInternalIP,
		aws.StringValue(d.instance.PrivateDnsName):   v1.NodeInternalDNS,
	}

	delete(addresses, "")

	return addresses
}

func (d *awsInstance) Status() instance.Status {
	switch *d.instance.State.Name {
	case ec2.InstanceStateNameRunning:
		return instance.StatusRunning
	case ec2.InstanceStateNamePending:
		return instance.StatusCreating
	case ec2.InstanceStateNameTerminated:
		return instance.StatusDeleted
	case ec2.InstanceStateNameShuttingDown:
		return instance.StatusDeleting
	default:
		return instance.StatusUnknown
	}
}

func getTagValue(name string, tags []*ec2.Tag) string {
	for _, t := range tags {
		if *t.Key == name {
			return *t.Value
		}
	}
	return ""
}

// awsErrorToTerminalError judges if the given error
// can be qualified as a "terminal" error, for more info see v1alpha1.MachineStatus
//
// if the given error doesn't qualify the error passed as
// an argument will be formatted according to msg and returned
func awsErrorToTerminalError(err error, msg string) error {
	prepareAndReturnError := func() error {
		return fmt.Errorf("%s, due to %s", msg, err)
	}

	if err != nil {
		aerr, ok := err.(awserr.Error)
		if !ok {
			return prepareAndReturnError()
		}
		switch aerr.Code() {
		case "InstanceLimitExceeded":
			return cloudprovidererrors.TerminalError{
				Reason:  common.InsufficientResourcesMachineError,
				Message: "You've reached the AWS quota for number of instances of this type",
			}
		case "AuthFailure":
			// authorization primitives come from MachineSpec
			// thus we are setting InvalidConfigurationMachineError
			return cloudprovidererrors.TerminalError{
				Reason:  common.InvalidConfigurationMachineError,
				Message: "A request has been rejected due to invalid credentials which were taken from the MachineSpec",
			}
		case "OptInRequired":
			// User has to accept the terms of the AMI
			return cloudprovidererrors.TerminalError{
				Reason:  "AMI terms not accepted",
				Message: err.Error(),
			}
		default:
			return prepareAndReturnError()
		}
	}
	return nil
}

func setProviderSpec(rawConfig awstypes.RawConfig, provSpec clusterv1alpha1.ProviderSpec) (*runtime.RawExtension, error) {
	if provSpec.Value == nil {
		return nil, fmt.Errorf("machine.spec.providerconfig.value is nil")
	}

	pconfig, err := providerconfigtypes.GetConfig(provSpec)
	if err != nil {
		return nil, err
	}

	rawCloudProviderSpec, err := json.Marshal(rawConfig)
	if err != nil {
		return nil, err
	}

	pconfig.CloudProviderSpec = runtime.RawExtension{Raw: rawCloudProviderSpec}
	rawPconfig, err := json.Marshal(pconfig)
	if err != nil {
		return nil, err
	}

	return &runtime.RawExtension{Raw: rawPconfig}, nil
}

func (p *provider) SetMetricsForMachines(machines clusterv1alpha1.MachineList) error {
	metricInstancesForMachines.Reset()

	if len(machines.Items) < 1 {
		return nil
	}

	type ec2Credentials struct {
		acccessKeyID         string
		secretAccessKey      string
		region               string
		assumeRoleARN        string
		assumeRoleExternalID string
	}

	var machineErrors []error
	machineEc2Credentials := map[string]ec2Credentials{}
	for _, machine := range machines.Items {
		config, _, _, err := p.getConfig(machines.Items[0].Spec.ProviderSpec)
		if err != nil {
			machineErrors = append(machineErrors, fmt.Errorf("failed to parse MachineSpec of machine %s/%s, due to %v", machine.Namespace, machine.Name, err))
			continue
		}

		// Very simple and very stupid
		machineEc2Credentials[fmt.Sprintf("%s/%s/%s/%s/%s", config.AccessKeyID, config.SecretAccessKey, config.Region, config.AssumeRoleARN, config.AssumeRoleExternalID)] = ec2Credentials{
			acccessKeyID:         config.AccessKeyID,
			secretAccessKey:      config.SecretAccessKey,
			region:               config.Region,
			assumeRoleARN:        config.AssumeRoleARN,
			assumeRoleExternalID: config.AssumeRoleExternalID,
		}
	}

	allReservations := []*ec2.Reservation{}
	for _, cred := range machineEc2Credentials {
		ec2Client, err := getEC2client(cred.acccessKeyID, cred.secretAccessKey, cred.region, cred.assumeRoleARN, cred.assumeRoleExternalID)
		if err != nil {
			machineErrors = append(machineErrors, fmt.Errorf("failed to get EC2 client: %v", err))
			continue
		}
		inOut, err := ec2Client.DescribeInstances(&ec2.DescribeInstancesInput{})
		if err != nil {
			machineErrors = append(machineErrors, fmt.Errorf("failed to get EC2 instances: %v", err))
			continue
		}
		allReservations = append(allReservations, inOut.Reservations...)
	}

	for _, machine := range machines.Items {
		metricInstancesForMachines.WithLabelValues(fmt.Sprintf("%s/%s", machine.Namespace, machine.Name)).Set(
			getIntanceCountForMachine(machine, allReservations))
	}

	if len(machineErrors) > 0 {
		return fmt.Errorf("errors: %v", machineErrors)
	}

	return nil
}

func getIntanceCountForMachine(machine clusterv1alpha1.Machine, reservations []*ec2.Reservation) float64 {
	var count float64
	for _, reservation := range reservations {
		for _, i := range reservation.Instances {
			if i.State == nil ||
				i.State.Name == nil ||
				*i.State.Name == ec2.InstanceStateNameTerminated {
				continue
			}

			for _, tag := range i.Tags {
				if *tag.Key != machineUIDTag {
					continue
				}

				if *tag.Value == string(machine.UID) {
					count = count + 1
				}
				break
			}
		}
	}
	return count
}

func filterSupportedRHELImages(images []*ec2.Image) ([]*ec2.Image, error) {
	var filteredImages []*ec2.Image
	for _, image := range images {
		if strings.HasPrefix(*image.Name, "RHEL-8") {
			filteredImages = append(filteredImages, image)
		}
	}

	if filteredImages == nil {
		return nil, errors.New("rhel 8 images are not found")
	}

	return filteredImages, nil
}

// waitForInstance waits for AWS instance to be created.
// If machine-controller tries to get an instance before it's fully-created,
// but after the instance request has been issued, it could
// happen that it detects that there's no instance and create it again.
// That could result in two or more instances created for one Machine object.
// This happens more often in some AWS regions because some regions have
// slower instance creation (e.g. us-east-1 and us-west-2).
func (p *provider) waitForInstance(machine *clusterv1alpha1.Machine) error {
	return wait.PollImmediate(pollInterval, pollTimeout, func() (bool, error) {
		_, err := p.get(machine)
		if err == cloudprovidererrors.ErrInstanceNotFound {
			// Retry if instance is not found
			return false, nil
		} else if err != nil {
			// If it's any error other then InstanceNotFound,
			// return the error and stop retrying.
			return false, err
		}

		return true, nil
	})
}
