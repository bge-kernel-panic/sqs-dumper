package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var (
	profile           string
	queue             string
	output            string
	loopCount         int32
	visibilityTimeout int32
	deleteMessages    bool
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&output, "output", "o", "./messages", "output directory")
	rootCmd.PersistentFlags().StringVarP(&profile, "profile", "p", "", "aws profile")
	rootCmd.PersistentFlags().StringVarP(&queue, "queue", "q", "", "queue url")
	rootCmd.PersistentFlags().Int32VarP(&loopCount, "loop-count", "", 1000, "number of loops for receive")
	rootCmd.PersistentFlags().Int32VarP(&visibilityTimeout, "visibility-timeout", "", 60, "visibility timeout in seconds")
	rootCmd.PersistentFlags().BoolVarP(&deleteMessages, "delete", "", false, "delete messages instead of setting visibility timeout")
}

var rootCmd = &cobra.Command{
	Use: "sqs-dumper",
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := os.Stat(output); os.IsNotExist(err) {
			if err := os.Mkdir(output, 0777); err != nil {
				return err
			}
		}

		sess, err := session.NewSessionWithOptions(session.Options{
			SharedConfigState: session.SharedConfigEnable,
			Profile:           profile,
		})

		if err != nil {
			return err
		}

		svc := sqs.New(sess)
		createdPrefixes := make(map[string]struct{})

		for i := 1; i <= int(loopCount); i++ {
			receiveResult, err := svc.ReceiveMessage(&sqs.ReceiveMessageInput{
				QueueUrl:            aws.String(queue),
				MaxNumberOfMessages: aws.Int64(10),
				MessageAttributeNames: []*string{
					aws.String(sqs.QueueAttributeNameAll),
				},
				WaitTimeSeconds:   aws.Int64(10),
				VisibilityTimeout: aws.Int64(int64(visibilityTimeout)),
			})

			if err != nil {
				return err
			}

			if deleteMessages && len(receiveResult.Messages) == 0 {
				fmt.Println("All messages consumed. Stop.")
				break
			}

			for _, message := range receiveResult.Messages {
				prefix := (*message.MessageId)[0:1]
				if _, ok := createdPrefixes[prefix]; !ok {
					_ = os.Mkdir(strings.TrimRight(output, "/")+"/"+prefix, 0777)
					createdPrefixes[prefix] = struct{}{}
				}
				path := strings.TrimRight(output, "/") + "/" + prefix + "/" + *message.MessageId + ".json"
				if _, err := os.Stat(path); os.IsNotExist(err) {
					if err := ioutil.WriteFile(path, []byte(*message.Body), 0644); err != nil {
						return err
					}
				}
			}

			if deleteMessages {
				batch := sqs.DeleteMessageBatchInput{
					QueueUrl: aws.String(queue),
					Entries:  transformMessages(receiveResult.Messages),
				}

				r, err := svc.DeleteMessageBatch(&batch)
				if err != nil {
					fmt.Printf("WARN: delete message failed: %v\n", err)
				}
				fmt.Printf("INFO: delete result: deleted %d messages, %d not deleted\n",
					len(r.Successful),
					len(r.Failed))
			}
		}

		return nil
	},
}

func transformMessages(in []*sqs.Message) []*sqs.DeleteMessageBatchRequestEntry {
	output := make([]*sqs.DeleteMessageBatchRequestEntry, len(in))
	for idx, message := range in {
		id := uuid.New().String()
		output[idx] = &sqs.DeleteMessageBatchRequestEntry{
			Id:            &id,
			ReceiptHandle: message.ReceiptHandle,
		}
	}
	return output
}
