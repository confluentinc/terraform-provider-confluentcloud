module github.com/confluentinc/terraform-provider-ccloud

go 1.15

require (
	github.com/antihax/optional v1.0.0
	github.com/confluentinc/ccloud-sdk-go-v2/cmk v0.3.0
	github.com/confluentinc/ccloud-sdk-go-v2/iam v0.4.0
	github.com/confluentinc/ccloud-sdk-go-v2/kafkarest v0.3.0
	github.com/confluentinc/ccloud-sdk-go-v2/mds v0.3.0
	github.com/confluentinc/ccloud-sdk-go-v2/org v0.4.0
	github.com/docker/go-connections v0.4.0
	github.com/hashicorp/go-cty v1.4.1-0.20200414143053-d3edf31b6320
	github.com/hashicorp/terraform-plugin-sdk/v2 v2.6.1
	github.com/stretchr/testify v1.7.0
	github.com/testcontainers/testcontainers-go v0.11.0
	github.com/walkerus/go-wiremock v1.2.0
)

replace (
	github.com/opencontainers/image-spec => github.com/opencontainers/image-spec v1.0.2
	github.com/opencontainers/runc => github.com/opencontainers/runc v1.0.3
	github.com/willf/bitset => github.com/bits-and-blooms/bitset v1.1.11
)
