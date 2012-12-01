// Public Domain (-) 2012 The Golly Authors.
// See the Golly UNLICENSE file for details.

// Package aws implements basic support for Amazon Web Services.
package aws

// Region encapsulates info relating to an AWS region.
type Region struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	DynamoDBEndpoint     string `json:"dynamodb-endpoint"`
	S3Endpoint           string `json:"s3-endpoint"`
	S3LocationConstraint string `json:"s3-location-constraint"`
}

func (r *Region) Clone() *Region {
	return &Region{
		ID:                   r.ID,
		Name:                 r.Name,
		DynamoDBEndpoint:     r.DynamoDBEndpoint,
		S3Endpoint:           r.S3Endpoint,
		S3LocationConstraint: r.S3LocationConstraint,
	}
}

func awsRegion(id, name string) *Region {
	r := &Region{ID: id, Name: name}
	Regions[id] = r
	Regions[name] = r
	switch id {
	case "us-east-1":
		r.S3Endpoint = "s3.amazonaws.com"
		r.S3LocationConstraint = ""
	case "eu-west-1":
		r.S3Endpoint = "s3-eu-west-1.amazonaws.com"
		r.S3LocationConstraint = "EU"
	default:
		r.S3Endpoint = "s3-" + r.ID + ".amazonaws.com"
		r.S3LocationConstraint = r.Name
	}
	r.DynamoDBEndpoint = "dynamodb." + r.ID + ".amazonaws.com"
	return r
}

var (
	APNorthEast1 = awsRegion("ap-northeast-1", "Tokyo")
	APSouthEast1 = awsRegion("ap-southeast-1", "Singapore")
	APSouthEast2 = awsRegion("ap-southeast-2", "Sydney")
	EUWest1      = awsRegion("eu-west-1", "Ireland")
	SAEast1      = awsRegion("sa-east-1", "Sao Paulo")
	USEast1      = awsRegion("us-east-1", "N. Virginia")
	USWest1      = awsRegion("us-west-1", "Oregon")
	USWest2      = awsRegion("us-west-2", "Northern California")
)

// Regions contains a mapping of region identifiers and names to Region structs.
var Regions = map[string]*Region{}
