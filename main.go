package main

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"io/ioutil"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
)

const (
	baseURL       = "http://codestar.me/"
	tableName     = "codestar.me"
	maxBodyLength = 500
)

var dynoClient *dynamodb.DynamoDB

type item struct {
	Ref    string `json:"ref,omitempty"`
	Body   string `json:"body,omitempty"`
	Expire int64  `json:"expire,omitempty"`
}

func main() {

	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	dynoClient = dynamodb.New(sess)

	lambda.Start(handler)
}

func handler(request events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {

	fmt.Printf("%v", request)
	method := request.RequestContext.HTTP.Method
	path := request.RequestContext.HTTP.Path
	switch method {
	case "GET":
		// if homepage. TODO: change integration -> route to S3
		if path == "/" {
			return index()
		}

		// getting single item by ref
		i, err := getItem(refFromPath(path))
		if err != nil {
			return errorResponse(err, "error getting item")
		}
		return validResponse(i.Body)

	case "POST":
		// take body

		body := strings.TrimSpace(request.Body)
		if len(body) > maxBodyLength {
			return errorResponse(fmt.Errorf("body limit error"), fmt.Sprintf("body too big. max %d characters", maxBodyLength))
		}
		ref := makeRef(body)

		err := saveItem(ref, body)
		if err != nil {
			return errorResponse(err, "error saving item")
		}

		return validResponse(baseURL + ref)
	default:
		return errorResponse(fmt.Errorf("not valid method "), "not valid method")
	}
}

func makeRef(body string) string {
	hash := sha1.New()
	hash.Write([]byte(body))
	return base64.URLEncoding.EncodeToString(hash.Sum(nil))[0:5]
}

// taking only first 5 characters from the path
func refFromPath(path string) string {
	return path[1:6]
}

func index() (events.APIGatewayV2HTTPResponse, error) {
	homepage, err := ioutil.ReadFile("public/index.html")
	if err != nil {
		return events.APIGatewayV2HTTPResponse{}, err
	}

	return events.APIGatewayV2HTTPResponse{
		StatusCode: 200,
		Body:       string(homepage),
		Headers: map[string]string{
			"Content-Type": "text/html",
		},
	}, nil
}

func saveItem(ref, body string) error {
	i := item{
		Ref:    ref,
		Body:   body,
		Expire: time.Now().Add(5 * time.Minute).Unix(), // expire after 5 minutes
	}

	marshaled, err := dynamodbattribute.MarshalMap(i)
	if err != nil {
		return fmt.Errorf("error marshalling new item: %w", err)
	}

	input := &dynamodb.PutItemInput{
		Item:      marshaled,
		TableName: aws.String(tableName),
	}

	_, err = dynoClient.PutItem(input)
	if err != nil {
		return fmt.Errorf("error calling PutItem: %w", err)
	}
	return nil
}

func getItem(ref string) (*item, error) {

	result, err := dynoClient.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]*dynamodb.AttributeValue{
			"ref": {
				S: aws.String(ref),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("error calling GetItem: %w", err)
	}

	item := new(item)
	err = dynamodbattribute.UnmarshalMap(result.Item, item)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling item: %w", err)
	}
	return item, nil
}

func errorResponse(err error, userMessage string) (events.APIGatewayV2HTTPResponse, error) {
	log.Println(err)
	return events.APIGatewayV2HTTPResponse{
		StatusCode: 500,
		Body:       userMessage,
		Headers: map[string]string{
			"Content-Type": "text/plain",
		},
	}, nil
}

func validResponse(body string) (events.APIGatewayV2HTTPResponse, error) {
	return events.APIGatewayV2HTTPResponse{
		StatusCode:      200,
		Body:            body,
		IsBase64Encoded: false,
		Headers: map[string]string{
			"Content-Type": "text/plain",
		},
	}, nil
}
