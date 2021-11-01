package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"os"
	"strconv"
	"time"
)

func panicErr(err error) {
	if err != nil {
		panic(err)
	}
}

func handler(ctx context.Context, request events.CloudWatchEvent) {
	roleArn := os.Getenv("ROLE_ARN")
	lifespanMinutes, err := strconv.Atoi(os.Getenv("LIFESPAN_MINUTES"))
	panicErr(err)

	cfg, err := config.LoadDefaultConfig(ctx)
	panicErr(err)
	client := cloudformation.NewFromConfig(cfg)

	describeInput := cloudformation.DescribeStacksInput{}
	describeOutput, err := client.DescribeStacks(ctx, &describeInput)
	panicErr(err)

	var targetList []string
	now := time.Now()

	for next := true; next; {
		for i := 0; i < len(describeOutput.Stacks); i++ {
			fmt.Printf("---\nName: %s\nCreation: %v\nStatus: %s\nUpdated: %v\nParent: %v\nRoot: %v\nID: %s\n", *describeOutput.Stacks[i].StackName, describeOutput.Stacks[i].CreationTime, describeOutput.Stacks[i].StackStatus, describeOutput.Stacks[i].LastUpdatedTime, describeOutput.Stacks[i].ParentId, describeOutput.Stacks[i].RootId, *describeOutput.Stacks[i].StackId)
			if len(describeOutput.Stacks[i].Tags) > 0 {
				fmt.Printf("Tags:\n")
				for ii := 0; ii < len(describeOutput.Stacks[i].Tags); ii++ {
					fmt.Printf("\t%s : %s\n", *describeOutput.Stacks[i].Tags[ii].Key, *describeOutput.Stacks[i].Tags[ii].Value)
				}
			}

			// Check if tags match
			tagMatch := false
			if len(describeOutput.Stacks[i].Tags) > 0 {
				for ii := 0; ii < len(describeOutput.Stacks[i].Tags); ii++ {
					if *describeOutput.Stacks[i].Tags[ii].Key == "cleanup" && *describeOutput.Stacks[i].Tags[ii].Value == "true" {
						tagMatch = true
					}
				}
			}

			// Check if state match
			stateMatch := false
			if describeOutput.Stacks[i].StackStatus == types.StackStatusUpdateComplete || describeOutput.Stacks[i].StackStatus == types.StackStatusCreateComplete {
				stateMatch = true
			}

			// Check if last modified older than time period
			timeMatch := false
			var lastModified time.Time
			if describeOutput.Stacks[i].LastUpdatedTime != nil {
				lastModified = *describeOutput.Stacks[i].LastUpdatedTime
			} else {
				lastModified = *describeOutput.Stacks[i].CreationTime
			}
			if int(now.Sub(lastModified).Minutes()) > lifespanMinutes {
				timeMatch = true
			}

			// Check if not a nested stack (don't delete nested stacks directly)
			notSubstack := false
			if describeOutput.Stacks[i].ParentId == nil && describeOutput.Stacks[i].RootId == nil {
				notSubstack = true
			}

			// All conditions match, add to target list
			if stateMatch && tagMatch && timeMatch && notSubstack {
				targetList = append(targetList, *describeOutput.Stacks[i].StackId)
			}
		}

		if describeOutput.NextToken == nil {
			next = false
		} else {
			describeInput = cloudformation.DescribeStacksInput{NextToken: describeOutput.NextToken}
			describeOutput, err = client.DescribeStacks(ctx, &describeInput)
			panicErr(err)
		}
	}

	if len(targetList) > 0 {
		for i := 0; i < len(targetList); i++ {
			fmt.Printf("***TARGET*** %s\n", targetList[i])

			deleteInput := cloudformation.DeleteStackInput{StackName: &targetList[i], RoleARN: aws.String(roleArn)}
			_, err := client.DeleteStack(ctx, &deleteInput)
			panicErr(err)
		}
	}
}

func main() {
	lambda.Start(handler)
}
