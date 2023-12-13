package network

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	cloudv1beta1 "github.com/redpanda-data/terraform-provider-redpanda/proto/gen/go/redpanda/api/controlplane/v1beta1"
	"github.com/redpanda-data/terraform-provider-redpanda/redpanda/clients"
	"github.com/redpanda-data/terraform-provider-redpanda/redpanda/models"
	"github.com/redpanda-data/terraform-provider-redpanda/redpanda/utils"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &Network{}
	_ resource.ResourceWithConfigure   = &Network{}
	_ resource.ResourceWithImportState = &Network{}
)

type Network struct {
	NetClient cloudv1beta1.NetworkServiceClient
	OpsClient cloudv1beta1.OperationServiceClient
}

func (n *Network) Metadata(_ context.Context, _ resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = "redpanda_network"
}

func (n *Network) Configure(ctx context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	if request.ProviderData == nil {
		// we can't add a diagnostic for an unset providerdata here because during the early part of the terraform
		// lifecycle, the provider data is not set and this is valid
		// but we also can't do anything until it is set
		response.Diagnostics.AddWarning("provider data not set", "provider data not set at network.Configure")
		return
	}

	p, ok := request.ProviderData.(utils.ResourceData)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *provider.Data, got: %T. Please report this issue to the provider developers.", request.ProviderData),
		)
		return
	}
	client, err := clients.NewNetworkServiceClient(ctx, p.Version, clients.ClientRequest{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
	})
	if err != nil {
		response.Diagnostics.AddError("failed to create network client", err.Error())
		return
	}

	opsClient, err := clients.NewOperationServiceClient(ctx, p.Version, clients.ClientRequest{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
	})
	if err != nil {
		response.Diagnostics.AddError("failed to create ops client", err.Error())
		return
	}

	n.NetClient = client
	n.OpsClient = opsClient
}

func (n *Network) Schema(_ context.Context, _ resource.SchemaRequest, response *resource.SchemaResponse) {
	response.Schema = ResourceNetworkSchema()
}

func ResourceNetworkSchema() schema.Schema {
	return schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:      true,
				Description:   "Name of the network",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"cidr_block": schema.StringAttribute{
				Required:      true,
				Description:   "The cidr_block to create the network in",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}\/(\d{1,2})$`),
						"The value must be a valid CIDR block (e.g., 192.168.0.0/16)",
					),
				},
			},
			"region": schema.StringAttribute{
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Description:   "The region to create the network in. Can also be set at the provider level",
			},
			"cloud_provider": schema.StringAttribute{
				Optional:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Description:   "The cloud provider to create the network in. Can also be set at the provider level",
				Validators: []validator.String{
					stringvalidator.OneOf("gcp", "aws"),
				},
			},
			"namespace_id": schema.StringAttribute{
				Required:      true,
				Description:   "The id of the namespace in which to create the network",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "UUID of the namespace",
			},
			"cluster_type": schema.StringAttribute{
				Required:    true,
				Description: "The type of cluster this network is associated with, can be one of dedicated or cloud",
				Validators: []validator.String{
					stringvalidator.OneOf("dedicated", "cloud"),
				},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
	}
}

func (n *Network) Create(ctx context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	var model models.Network
	response.Diagnostics.Append(request.Plan.Get(ctx, &model)...)

	cloudProvider := utils.StringToCloudProvider(model.CloudProvider.ValueString())
	// TODO add a check to the provider data here to see if region and cloud provider are set
	// prefer the local value, but accept the provider value if local is unavailable
	// if neither are set, fail

	op, err := n.NetClient.CreateNetwork(ctx, &cloudv1beta1.CreateNetworkRequest{
		Network: &cloudv1beta1.Network{
			Name:          model.Name.ValueString(),
			CidrBlock:     model.CidrBlock.ValueString(),
			Region:        model.Region.ValueString(),
			CloudProvider: cloudProvider,
			NamespaceId:   model.NamespaceID.ValueString(),
			ClusterType:   utils.StringToClusterType(model.ClusterType.ValueString()),
		},
	})
	if err != nil {
		response.Diagnostics.AddError("failed to create network", err.Error())
		return
	}
	var metadata cloudv1beta1.CreateNetworkMetadata
	if err := op.Metadata.UnmarshalTo(&metadata); err != nil {
		response.Diagnostics.AddError("failed to unmarshal network metadata", err.Error())
		return
	}

	// TODO accept user configuration for timeout
	if err := utils.AreWeDoneYet(ctx, op, 15*time.Minute, n.OpsClient); err != nil {
		response.Diagnostics.AddError("failed waiting for network creation", err.Error())
		return
	}

	response.Diagnostics.Append(response.State.Set(ctx, models.Network{
		Name:          model.Name,
		ID:            utils.TrimmedStringValue(metadata.GetNetworkId()),
		CidrBlock:     model.CidrBlock,
		Region:        model.Region,
		NamespaceID:   model.NamespaceID,
		ClusterType:   model.ClusterType,
		CloudProvider: model.CloudProvider,
	})...)
}

func (n *Network) Read(ctx context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	var model models.Network
	response.Diagnostics.Append(request.State.Get(ctx, &model)...)
	nw, err := n.NetClient.GetNetwork(ctx, &cloudv1beta1.GetNetworkRequest{
		Id: model.ID.ValueString(),
	})
	if err != nil {
		if utils.IsNotFound(err) {
			response.State.RemoveResource(ctx)
			return
		} else {
			response.Diagnostics.AddError(fmt.Sprintf("failed to read network %s", model.ID.ValueString()), err.Error())
			return
		}
	}
	response.Diagnostics.Append(response.State.Set(ctx, models.Network{
		Name:          types.StringValue(nw.Name),
		ID:            types.StringValue(nw.Id),
		CidrBlock:     types.StringValue(nw.CidrBlock),
		Region:        types.StringValue(nw.Region),
		NamespaceID:   types.StringValue(nw.NamespaceId),
		CloudProvider: types.StringValue(utils.CloudProviderToString(nw.CloudProvider)),
		ClusterType:   types.StringValue(utils.ClusterTypeToString(nw.ClusterType)),
	})...)
}

// Update is not supported for network. As a result all configurable schema elements have been marked as RequiresReplace
func (n *Network) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

func (n *Network) Delete(ctx context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	var model models.Network
	response.Diagnostics.Append(request.State.Get(ctx, &model)...)
	op, err := n.NetClient.DeleteNetwork(ctx, &cloudv1beta1.DeleteNetworkRequest{
		Id: model.ID.ValueString(),
	})
	if err != nil {
		response.Diagnostics.AddError("failed to delete network", err.Error())
		return
	}
	// TODO allow configurable timeout
	if err := utils.AreWeDoneYet(ctx, op, 15*time.Minute, n.OpsClient); err != nil {
		response.Diagnostics.AddError("failed waiting for network deletion", err.Error())
	}
}

// ImportState refreshes the state with the correct ID for the namespace, allowing TF to use Read to get the correct Namespace name into state
// see https://developer.hashicorp.com/terraform/plugin/framework/resources/import for more details
func (n *Network) ImportState(ctx context.Context, request resource.ImportStateRequest, response *resource.ImportStateResponse) {
	response.Diagnostics.Append(response.State.Set(ctx, models.Network{
		ID: types.StringValue(request.ID),
	})...)
}
