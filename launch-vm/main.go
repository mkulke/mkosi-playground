// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See License.txt in the project root for license information.

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

var (
	resourceGroupName string
	subnetID          string
	imageID           string
	subscriptionID    string
	keep              bool
	secureBoot        bool
	sshPublicKeyPath  string
	location          string
	instanceSize      string
	confidential      bool
	vmName            string
)

var (
	resourcesClientFactory *armresources.ClientFactory
	computeClientFactory   *armcompute.ClientFactory
	networkClientFactory   *armnetwork.ClientFactory
)

var (
	resourceGroupClient   *armresources.ResourceGroupsClient
	interfacesClient      *armnetwork.InterfacesClient
	virtualMachinesClient *armcompute.VirtualMachinesClient
	disksClient           *armcompute.DisksClient
)

type VMResources struct {
	vmName   string
	nicName  string
	diskName string
}

func main() {
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "resource-group",
				Aliases:     []string{"g"},
				Usage:       "resource group name",
				EnvVars:     []string{"RESOURCE_GROUP"},
				Destination: &resourceGroupName,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "subnet-id",
				Aliases:     []string{"n"},
				Usage:       "subnet id to attach the nic to",
				EnvVars:     []string{"SUBNET_ID"},
				Destination: &subnetID,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "image-id",
				Aliases:     []string{"i"},
				Usage:       "image id to use for the virtual machine",
				EnvVars:     []string{"IMAGE_ID"},
				Destination: &imageID,
				Required:    true,
			},
			&cli.StringFlag{
				Name:        "subscription-id",
				Aliases:     []string{"s"},
				Usage:       "subscription id",
				EnvVars:     []string{"SUBSCRIPTION_ID"},
				Destination: &subscriptionID,
				Required:    true,
			},
			&cli.BoolFlag{
				Name:        "secure-boot",
				Aliases:     []string{"b"},
				Usage:       "enable secure boot for the virtual machine",
				Destination: &secureBoot,
			},
			&cli.BoolFlag{
				Name:        "confidential",
				Aliases:     []string{"c"},
				Usage:       "enable confidential computing for the virtual machine",
				Destination: &confidential,
			},
			&cli.StringFlag{
				Name:        "location",
				Aliases:     []string{"l"},
				Usage:       "location of the virtual machine",
				EnvVars:     []string{"LOCATION"},
				Destination: &location,
				Value:       "westeurope",
			},
			&cli.StringFlag{
				Name:        "name",
				Usage:       "name of the virtual machine",
				EnvVars:     []string{"VM_NAME"},
				Destination: &vmName,
				Value:       "test-vm",
			},
			&cli.StringFlag{
				Name:        "size",
				Aliases:     []string{"z"},
				Usage:       "instance size of the virtual machine",
				EnvVars:     []string{"INSTANCE_SIZE"},
				Destination: &instanceSize,
				DefaultText: "Standard_DC2as_v5",
			},
			&cli.BoolFlag{
				Name:        "keep",
				Aliases:     []string{"k"},
				Usage:       "do not delete the resources after creating them",
				Destination: &keep,
			},
			&cli.StringFlag{
				Name:        "pub-key",
				Aliases:     []string{"p"},
				Usage:       "ssh public key path",
				EnvVars:     []string{"SSH_PUBLIC_KEY"},
				Destination: &sshPublicKeyPath,
				DefaultText: "~/.ssh/id_ed25519.pub",
			},
		},
		Action: func(cCtx *cli.Context) error {
			resources, err := createVM()
			if err != nil {
				return err
			}
			if keep == true {
				return nil
			}
			return cleanup(resources)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func buildNetworkConfig(nicName string) *armcompute.VirtualMachineNetworkInterfaceConfiguration {
	ipConfig := armcompute.VirtualMachineNetworkInterfaceIPConfiguration{
		Name: to.Ptr("ip-config"),
		Properties: &armcompute.VirtualMachineNetworkInterfaceIPConfigurationProperties{
			Subnet: &armcompute.SubResource{
				ID: to.Ptr(subnetID),
			},
		},
	}

	config := armcompute.VirtualMachineNetworkInterfaceConfiguration{
		Name: to.Ptr(nicName),
		Properties: &armcompute.VirtualMachineNetworkInterfaceConfigurationProperties{
			DeleteOption:     to.Ptr(armcompute.DeleteOptionsDelete),
			IPConfigurations: []*armcompute.VirtualMachineNetworkInterfaceIPConfiguration{&ipConfig},
		},
	}
	return &config
}

func buildSSHConfig(userName string) (*armcompute.SSHConfiguration, error) {
	config := armcompute.SSHConfiguration{
		PublicKeys: []*armcompute.SSHPublicKey{},
	}
	path := sshPublicKeyPath
	if path == "" {
		homedir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = homedir + "/.ssh/id_ed25519.pub"
	}
	var bytes []byte
	_, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	bytes, err = os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	remotePath := fmt.Sprintf("/home/%s/.ssh/authorized_keys", userName)
	key := armcompute.SSHPublicKey{
		Path:    to.Ptr(remotePath),
		KeyData: to.Ptr(string(bytes)),
	}
	config.PublicKeys = append(config.PublicKeys, &key)
	return &config, nil
}

func buildImageRef(id string) *armcompute.ImageReference {
	ref := armcompute.ImageReference{}
	if strings.HasPrefix(id, "/CommunityGalleries") {
		ref.CommunityGalleryImageID = to.Ptr(id)
	} else {
		ref.ID = to.Ptr(id)
	}
	return &ref
}

func createVM() (*VMResources, error) {
	conn, err := connectionAzure()
	if err != nil {
		return nil, err
	}
	ctx := context.Background()

	networkClientFactory, err = armnetwork.NewClientFactory(subscriptionID, conn, nil)
	if err != nil {
		return nil, err
	}
	interfacesClient = networkClientFactory.NewInterfacesClient()

	computeClientFactory, err = armcompute.NewClientFactory(subscriptionID, conn, nil)
	if err != nil {
		return nil, err
	}
	virtualMachinesClient = computeClientFactory.NewVirtualMachinesClient()
	disksClient = computeClientFactory.NewDisksClient()

	log.Println("start creating virtual machine...")
	nicName := vmName + "-nic"
	diskName := vmName + "-disk"

	virtualMachine, err := createVirtualMachine(ctx, nicName, diskName, vmName)
	if err != nil {
		return nil, err
	}
	log.Printf("created virtual machine: %s", *virtualMachine.ID)

	log.Println("virtual machine created successfully")

	return &VMResources{
		vmName:   vmName,
		nicName:  nicName,
		diskName: diskName,
	}, nil
}

func cleanup(resources *VMResources) error {
	ctx := context.Background()

	log.Println("start deleting virtual machine...")
	err := deleteVirtualMachine(ctx, resources.vmName)
	if err != nil {
		return err
	}
	log.Println("deleted virtual machine")

	err = deleteDisk(ctx, resources.diskName)
	if err != nil {
		return err
	}
	log.Println("deleted disk")

	err = deleteNetWorkInterface(ctx, resources.nicName)
	if err != nil {
		return err
	}
	log.Println("deleted network interface")

	log.Println("virtual machine deleted successfully")

	return nil
}

func connectionAzure() (azcore.TokenCredential, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}
	return cred, nil
}

func deleteNetWorkInterface(ctx context.Context, name string) error {
	pollerResponse, err := interfacesClient.BeginDelete(ctx, resourceGroupName, name, nil)
	if err != nil {
		return err
	}

	_, err = pollerResponse.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}

	return nil
}

func createVirtualMachine(ctx context.Context, nicName string, diskName string, name string) (*armcompute.VirtualMachine, error) {
	userName := "azureuser"

	sshConfig, err := buildSSHConfig(userName)
	if err != nil {
		return nil, err
	}

	networkConfig := buildNetworkConfig(nicName)

	securityProfile := &armcompute.SecurityProfile{
		SecurityType: to.Ptr(armcompute.SecurityTypesTrustedLaunch),
		UefiSettings: &armcompute.UefiSettings{
			SecureBootEnabled: to.Ptr(secureBoot),
			VTpmEnabled:       to.Ptr(true),
		},
	}

	osDiskParams := &armcompute.OSDisk{
		Name:         to.Ptr(diskName),
		CreateOption: to.Ptr(armcompute.DiskCreateOptionTypesFromImage),
		Caching:      to.Ptr(armcompute.CachingTypesReadWrite),
		DeleteOption: to.Ptr(armcompute.DiskDeleteOptionTypesDelete),
		ManagedDisk: &armcompute.ManagedDiskParameters{
			StorageAccountType: to.Ptr(armcompute.StorageAccountTypesStandardLRS),
		},
	}

	if confidential {
		securityProfile.SecurityType = to.Ptr(armcompute.SecurityTypesConfidentialVM)
		osDiskParams.ManagedDisk.SecurityProfile = &armcompute.VMDiskSecurityProfile{
			SecurityEncryptionType: to.Ptr(armcompute.SecurityEncryptionTypesNonPersistedTPM),
		}
	}

	parameters := armcompute.VirtualMachine{
		Location: to.Ptr(location),
		Identity: &armcompute.VirtualMachineIdentity{
			Type: to.Ptr(armcompute.ResourceIdentityTypeNone),
		},
		Properties: &armcompute.VirtualMachineProperties{
			StorageProfile: &armcompute.StorageProfile{
				ImageReference: buildImageRef(imageID),
				OSDisk:         osDiskParams,
			},
			HardwareProfile: &armcompute.HardwareProfile{
				VMSize: to.Ptr(armcompute.VirtualMachineSizeTypes(instanceSize)),
			},
			OSProfile: &armcompute.OSProfile{
				ComputerName:  to.Ptr(name),
				AdminUsername: to.Ptr(userName),
				LinuxConfiguration: &armcompute.LinuxConfiguration{
					DisablePasswordAuthentication: to.Ptr(true),
					SSH:                           sshConfig,
				},
			},
			SecurityProfile: securityProfile,
			DiagnosticsProfile: &armcompute.DiagnosticsProfile{
				BootDiagnostics: &armcompute.BootDiagnostics{
					Enabled: to.Ptr(true),
				},
			},
			NetworkProfile: &armcompute.NetworkProfile{
				NetworkAPIVersion:              to.Ptr(armcompute.NetworkAPIVersionTwoThousandTwenty1101),
				NetworkInterfaceConfigurations: []*armcompute.VirtualMachineNetworkInterfaceConfiguration{networkConfig},
			},
		},
	}

	pollerResponse, err := virtualMachinesClient.BeginCreateOrUpdate(ctx, resourceGroupName, name, parameters, nil)
	if err != nil {
		return nil, err
	}

	resp, err := pollerResponse.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}

	return &resp.VirtualMachine, nil
}

func deleteVirtualMachine(ctx context.Context, name string) error {
	pollerResponse, err := virtualMachinesClient.BeginDelete(ctx, resourceGroupName, name, nil)
	if err != nil {
		return err
	}

	_, err = pollerResponse.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}

	return nil
}

func deleteDisk(ctx context.Context, name string) error {

	pollerResponse, err := disksClient.BeginDelete(ctx, resourceGroupName, name, nil)
	if err != nil {
		return err
	}

	_, err = pollerResponse.PollUntilDone(ctx, nil)
	if err != nil {
		return err
	}
	return nil
}
