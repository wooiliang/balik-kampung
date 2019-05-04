package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/sns"
)

const url = "http://booking.balikkampung.co/index.php?controller=pjBusScheduleFront&action=pjActionCheck&hide=0&date=%v&pickup_id=10&return_id=15&is_return=F&return_date=%v&template=template_2"

func notify(date string) error {
	svc := sns.New(session.New(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
	}))
	input := &sns.PublishInput{
		Message:  aws.String("http://booking.balikkampung.co/"),
		Subject:  aws.String(fmt.Sprintf("%v ticket is available now!", date)),
		TopicArn: aws.String(fmt.Sprintf("arn:aws:sns:%v:%v:balik-kampung", os.Getenv("AWS_REGION"), os.Getenv("AWS_ACCOUNT_ID"))),
	}
	_, err := svc.Publish(input)
	if err != nil {
		return err
	}
	return nil
}

func formatDate(date string) string {
	s := strings.Split(date, "-")
	return fmt.Sprintf("%v.%v.%v", s[2], s[1], s[0])
}

func request(date string) (string, error) {
	d := formatDate(date)
	client := &http.Client{}
	req, err := http.NewRequest("GET", fmt.Sprintf(url, d, d), nil)
	req.Header.Add("X-Requested-With", "XMLHttpRequest")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("request failed")
	}
	body, err := ioutil.ReadAll(resp.Body)
	return string(body), nil
}

func isOpen(date string) (bool, error) {
	body, err := request(date)
	if err != nil {
		return false, err
	}
	if body == `{"code":200}` {
		return true, nil
	}
	return false, nil
}

func getLastDate() (string, error) {
	svc := dynamodb.New(session.New(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
	}))
	input := &dynamodb.QueryInput{
		ExpressionAttributeNames: map[string]*string{
			"#date": aws.String("date"),
			"#type": aws.String("type"),
		},
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":type": {
				S: aws.String("last"),
			},
		},
		KeyConditionExpression: aws.String("#type = :type"),
		Limit:                  aws.Int64(1),
		ProjectionExpression:   aws.String("#date"),
		ScanIndexForward:       aws.Bool(false),
		TableName:              aws.String("balik-kampung"),
	}
	result, err := svc.Query(input)
	if err != nil {
		return "", err
	}
	if len(result.Items) == 0 {
		return "", fmt.Errorf("no date found")
	}
	var obj []struct{ Date string }
	err = dynamodbattribute.UnmarshalListOfMaps(result.Items, &obj)
	if err != nil {
		return "", err
	}
	return obj[0].Date, nil
}

func getNextDate(date string) string {
	loc, _ := time.LoadLocation("Asia/Singapore")
	t, _ := time.ParseInLocation("2006-01-02", date, loc)
	d, _ := time.ParseDuration("168h")
	return t.Add(d).Format("2006-01-02")
}

func setLastDate(date string) error {
	svc := dynamodb.New(session.New(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
	}))
	input := &dynamodb.PutItemInput{
		Item: map[string]*dynamodb.AttributeValue{
			"type": {
				S: aws.String("last"),
			},
			"date": {
				S: aws.String(date),
			},
		},
		TableName: aws.String("balik-kampung"),
	}
	_, err := svc.PutItem(input)
	if err != nil {
		return err
	}
	return nil
}

// Handler is our lambda handler invoked by the `lambda.Start` function call
func Handler(ctx context.Context) (string, error) {
	date, err := getLastDate()
	if err != nil {
		return "", err
	}
	nextDate := getNextDate(date)
	isOpen, err := isOpen(nextDate)
	if err != nil {
		return "", err
	}
	if isOpen {
		if err := setLastDate(nextDate); err != nil {
			return "", err
		}
		if err := notify(nextDate); err != nil {
			return "", err
		}
		return fmt.Sprintf("%v ticket is available now!", nextDate), nil
	}
	return fmt.Sprintf("%v not yet open. Be patient.", nextDate), nil
}

func main() {
	lambda.Start(Handler)
}
