package stack

import (
	"fmt"
	"strconv"

	"github.com/apex/up"
	"github.com/apex/up/internal/util"
)

// Map .
type Map map[string]interface{}

// ref of id.
func ref(id string) Map {
	return Map{
		"Ref": id,
	}
}

// get value from named ref.
func get(name, value string) Map {
	return Map{
		"Fn::GetAtt": []string{
			name,
			value,
		},
	}
}

// join strings with delim.
func join(delim string, s ...interface{}) Map {
	return Map{
		"Fn::Join": []interface{}{
			delim,
			s,
		},
	}
}

// stageVariable by name.
func stageVariable(name string) string {
	return fmt.Sprintf("${stageVariables.%s}", name)
}

// lambda ARN for function name.
func lambdaArn(name string) Map {
	return join(":", "arn", "aws", "lambda", ref("AWS::Region"), ref("AWS::AccountId"), "function", ref(name))
}

// lambda ARN for function name with qualifier.
func lambdaArnQualifier(name, qualifier string) Map {
	return join(":", "arn", "aws", "lambda", ref("AWS::Region"), ref("AWS::AccountId"), "function", join(":", ref(name), qualifier))
}

// DNS resources.
func dns(c *up.Config, m Map) {
	for _, z := range c.DNS.Zones {
		zoneID := util.Camelcase("dns_zone_%s", z.Name)

		m[zoneID] = Map{
			"Type": "AWS::Route53::HostedZone",
			"Properties": Map{
				"Name": z.Name,
			},
		}

		for _, r := range z.Records {
			id := util.Camelcase("dns_zone_%s_record_%s", z.Name, r.Name)

			m[id] = Map{
				"Type": "AWS::Route53::RecordSet",
				"Properties": Map{
					"Name":            r.Name,
					"Type":            r.Type,
					"TTL":             strconv.Itoa(r.TTL),
					"ResourceRecords": r.Value,
					"HostedZoneId":    ref(zoneID),
				},
			}
		}
	}
}

// API resources.
func api(c *up.Config, m Map) {
	desc := util.ManagedByUp(c.Description)

	m["Api"] = Map{
		"Type": "AWS::ApiGateway::RestApi",
		"Properties": Map{
			"Name":        ref("Name"),
			"Description": desc,
			"BinaryMediaTypes": []string{
				"*/*",
			},
		},
	}

	integration := Map{
		"Type":                  "AWS_PROXY",
		"IntegrationHttpMethod": "POST",
		"Uri": join("",
			"arn:aws:apigateway:",
			ref("AWS::Region"),
			":lambda:path/2015-03-31/functions/",
			lambdaArnQualifier("FunctionName", stageVariable("qualifier")),
			"/invocations"),
	}

	m["ApiRootMethod"] = Map{
		"Type": "AWS::ApiGateway::Method",
		"Properties": Map{
			"RestApiId":         ref("Api"),
			"ResourceId":        get("Api", "RootResourceId"),
			"HttpMethod":        "ANY",
			"AuthorizationType": "NONE",
			"Integration":       integration,
		},
	}

	m["ApiProxyResource"] = Map{
		"Type": "AWS::ApiGateway::Resource",
		"Properties": Map{
			"RestApiId": ref("Api"),
			"ParentId":  get("Api", "RootResourceId"),
			"PathPart":  "{proxy+}",
		},
	}

	m["ApiProxyMethod"] = Map{
		"Type": "AWS::ApiGateway::Method",
		"Properties": Map{
			"RestApiId":         ref("Api"),
			"ResourceId":        ref("ApiProxyResource"),
			"HttpMethod":        "ANY",
			"AuthorizationType": "NONE",
			"Integration":       integration,
		},
	}

	// TODO: allow mapping in config
	m["ApiDeploymentDevelopment"] = Map{
		"Type":      "AWS::ApiGateway::Deployment",
		"DependsOn": []string{"ApiRootMethod", "ApiProxyMethod", "ApiFunctionAliasDevelopment"},
		"Properties": Map{
			"RestApiId": ref("Api"),
			"StageName": "development",
			"StageDescription": Map{
				"Variables": Map{
					"qualifier": "development",
				},
			},
		},
	}

	m["ApiDeploymentStaging"] = Map{
		"Type":      "AWS::ApiGateway::Deployment",
		"DependsOn": []string{"ApiRootMethod", "ApiProxyMethod", "ApiFunctionAliasStaging"},
		"Properties": Map{
			"RestApiId": ref("Api"),
			"StageName": "staging",
			"StageDescription": Map{
				"Variables": Map{
					"qualifier": "staging",
				},
			},
		},
	}

	m["ApiDeploymentProduction"] = Map{
		"Type":      "AWS::ApiGateway::Deployment",
		"DependsOn": []string{"ApiRootMethod", "ApiProxyMethod", "ApiFunctionAliasProduction"},
		"Properties": Map{
			"RestApiId": ref("Api"),
			"StageName": "production",
			"StageDescription": Map{
				"Variables": Map{
					"qualifier": "production",
				},
			},
		},
	}

	m["ApiFunctionAliasDevelopment"] = Map{
		"Type": "AWS::Lambda::Alias",
		"Properties": Map{
			"Name":            "development",
			"Description":     util.ManagedByUp("Development environment"),
			"FunctionName":    ref("FunctionName"),
			"FunctionVersion": "$LATEST",
		},
	}

	m["ApiFunctionAliasStaging"] = Map{
		"Type": "AWS::Lambda::Alias",
		"Properties": Map{
			"Name":            "staging",
			"Description":     util.ManagedByUp("Staging environment"),
			"FunctionName":    ref("FunctionName"),
			"FunctionVersion": ref("FunctionVersionStaging"),
		},
	}

	m["ApiFunctionAliasProduction"] = Map{
		"Type": "AWS::Lambda::Alias",
		"Properties": Map{
			"Name":            "production",
			"Description":     util.ManagedByUp("Production environment"),
			"FunctionName":    ref("FunctionName"),
			"FunctionVersion": ref("FunctionVersionProduction"),
		},
	}
}

// IAM resources.
func iam(c *up.Config, m Map) {
	m["ApiLambdaPermissionDevelopment"] = Map{
		"Type":      "AWS::Lambda::Permission",
		"DependsOn": "ApiFunctionAliasDevelopment",
		"Properties": Map{
			"Action":       "lambda:invokeFunction",
			"FunctionName": lambdaArnQualifier("FunctionName", "development"),
			"Principal":    "apigateway.amazonaws.com",
			"SourceArn": join("",
				"arn:aws:execute-api",
				":",
				ref("AWS::Region"),
				":",
				ref("AWS::AccountId"),
				":",
				ref("Api"),
				"/*"),
		},
	}

	m["ApiLambdaPermissionStaging"] = Map{
		"Type":      "AWS::Lambda::Permission",
		"DependsOn": "ApiFunctionAliasStaging",
		"Properties": Map{
			"Action":       "lambda:invokeFunction",
			"FunctionName": lambdaArnQualifier("FunctionName", "staging"),
			"Principal":    "apigateway.amazonaws.com",
			"SourceArn": join("",
				"arn:aws:execute-api",
				":",
				ref("AWS::Region"),
				":",
				ref("AWS::AccountId"),
				":",
				ref("Api"),
				"/*"),
		},
	}

	m["ApiLambdaPermissionProduction"] = Map{
		"Type":      "AWS::Lambda::Permission",
		"DependsOn": "ApiFunctionAliasProduction",
		"Properties": Map{
			"Action":       "lambda:invokeFunction",
			"FunctionName": lambdaArnQualifier("FunctionName", "production"),
			"Principal":    "apigateway.amazonaws.com",
			"SourceArn": join("",
				"arn:aws:execute-api",
				":",
				ref("AWS::Region"),
				":",
				ref("AWS::AccountId"),
				":",
				ref("Api"),
				"/*"),
		},
	}
}

// ACM resources.
func acm(c *up.Config, m Map) {
	for _, c := range c.Certs {
		domain := c.Domains[0]
		alts := c.Domains[1:]
		name := util.Camelcase("cert_%s", domain)

		props := Map{
			"DomainName": domain,
		}

		if len(alts) > 0 {
			props["SubjectAlternativeNames"] = alts
		}

		m[name] = Map{
			"Type":       "AWS::CertificateManager::Certificate",
			"Properties": props,
		}
	}
}

// resources of the stack.
func resources(c *up.Config) Map {
	m := Map{}

	api(c, m)
	iam(c, m)
	acm(c, m)
	dns(c, m)

	return m
}

// parameters of the stack.
func parameters(c *up.Config) Map {
	return Map{
		"Name": Map{
			"Description": "Name of application",
			"Type":        "String",
		},
		"FunctionName": Map{
			"Description": "Name of application function",
			"Type":        "String",
		},
		"FunctionVersionStaging": Map{
			"Description": "Version of staging deployment",
			"Type":        "String",
		},
		"FunctionVersionProduction": Map{
			"Description": "Version of production deployment",
			"Type":        "String",
		},
	}
}

// outputs of the stack.
func outputs(c *up.Config) Map {
	return Map{
		"ApiName": Map{
			"Description": "API name",
			"Value":       ref("Name"),
		},
		"ApiFunctionName": Map{
			"Description": "API Lambda function name",
			"Value":       ref("FunctionName"),
		},
		"ApiFunctionArn": Map{
			"Description": "API Lambda function ARN",
			"Value":       lambdaArn("FunctionName"),
		},
	}
}

// template for the given config.
func template(c *up.Config) Map {
	return Map{
		"AWSTemplateFormatVersion": "2010-09-09",
		"Parameters":               parameters(c),
		"Outputs":                  outputs(c),
		"Resources":                resources(c),
	}
}
