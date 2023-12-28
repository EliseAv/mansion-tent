package tower

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/joho/godotenv"
)

type dispatcher struct {
	aws      *session.Session
	ec2      *ec2.EC2
	r53      *route53.Route53
	s3       *s3.S3
	s3folder url.URL
	userdata *string
	trying   bool
	err      error
	ip       string
}

var (
	Dispatcher dispatcher

	ErrAlreadyRunning  = errors.New("instance already running")
	ErrNoAMI           = errors.New("no AMI found")
	ErrNoSecurityGroup = errors.New("no security group found")
)

func init() {
	session := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))
	Dispatcher.aws = session
	session.Config.Region = aws.String(os.Getenv("AWS_REGION"))
	Dispatcher.ec2 = ec2.New(session)
	Dispatcher.r53 = route53.New(session)
	Dispatcher.s3 = s3.New(session)

	parsed, err := url.Parse(os.Getenv("S3_FOLDER_URL"))
	if err != nil {
		panic(err)
	}
	parsed.Path = strings.Trim(parsed.Path, "/")
	Dispatcher.s3folder = *parsed
	Dispatcher.s3folder.Path = strings.Trim(Dispatcher.s3folder.Path, "/")
	Dispatcher.userdata = Dispatcher.generateUserData()

	Dispatcher.uploadSupportingFiles()
}

func (l *dispatcher) ConsoleLaunch() {
	l.LaunchFactorio()
	if l.err != nil {
		fmt.Printf("Launcher error: %s\n", l.err)
	} else {
		fmt.Printf("Launched at %s aka %s\n", os.Getenv("ROUTE53_FQDN"), l.ip)
	}
}

func (l *dispatcher) uploadSupportingFiles() {
	// get the directory of the executable
	executable, err := os.Executable()
	if err != nil {
		panic(err)
	}
	executableDir := filepath.Dir(executable)
	// upload the files
	filesToUpload := []string{"mt.x86", ".env"}
	for _, file := range filesToUpload {
		file, err := os.Open(executableDir + "/" + file)
		if err != nil {
			panic(err)
		}
		err = l.UploadToS3(file.Name(), file)
		if err != nil {
			panic(err)
		}
		fmt.Printf("Uploaded %s\n", file.Name())
	}
}

func (l *dispatcher) LaunchFactorio() {
	defer func() {
		l.err = recover().(error)
		if l.err != nil {
			debug.PrintStack()
		}
	}()
	if l.trying {
		panic(ErrAlreadyRunning)
	}
	l.trying = true
	l.checkIfAlreadyRunning()
	l.createInstance()
	l.updateDnsRecord()
	l.trying = false
}

func (l *dispatcher) getLatestAmazonLinuxAMI() *ec2.Image {
	params := &ec2.DescribeImagesInput{Filters: []*ec2.Filter{{
		Name:   aws.String("name"),
		Values: []*string{aws.String("al2023-ami-2*-x86_64")},
	}, {
		Name:   aws.String("owner-id"),
		Values: []*string{aws.String("137112412989")}, // Amazon
	}}}
	resp, err := l.ec2.DescribeImages(params)
	if err != nil {
		panic(err)
	}
	if len(resp.Images) == 0 {
		panic(ErrNoAMI)
	}
	// find the latest image
	latestAmi := resp.Images[0]
	for _, image := range resp.Images[1:] {
		if *latestAmi.CreationDate < *image.CreationDate {
			latestAmi = image
		}
	}
	return latestAmi
}

func (l *dispatcher) getDefaultSecurityGroup() *ec2.SecurityGroup {
	params := &ec2.DescribeSecurityGroupsInput{
		GroupNames: []*string{aws.String("default")},
	}
	resp, err := l.ec2.DescribeSecurityGroups(params)
	if err != nil {
		panic(err)
	}
	if len(resp.SecurityGroups) == 0 {
		panic(ErrNoSecurityGroup)
	}
	return resp.SecurityGroups[0]
}

func (l *dispatcher) generateUserData() *string {
	url := os.Getenv("S3_FOLDER_URL")
	values, err := godotenv.Read()
	if err != nil {
		panic(err)
	}
	values["TENT_MODE"] = "launch"
	marshalled, err := godotenv.Marshal(values)
	if err != nil {
		panic(err)
	}
	lines := "#!/bin/bash\n" +
		"mkdir -p /opt/mansionTent\n" +
		"aws s3 cp " + url + "/mansionTent /opt/mansionTent/mansionTent\n" +
		"chmod +x /opt/mansionTent/mansionTent\n" +
		"cat << EOF > /opt/mansionTent/.env\n" +
		marshalled + "\nEOF\n" +
		"sudo -iu ec2-user screen -dm /opt/mansionTent/mansionTent\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(lines))
	return &encoded
}

func (l *dispatcher) createInstance() {
	params := &ec2.RunInstancesInput{
		ImageId:          l.getLatestAmazonLinuxAMI().ImageId,
		InstanceType:     aws.String(os.Getenv("EC2_INSTANCE_TYPE")),
		MinCount:         aws.Int64(1),
		MaxCount:         aws.Int64(1),
		SecurityGroupIds: []*string{l.getDefaultSecurityGroup().GroupId},
		UserData:         l.userdata,
		DryRun:           aws.Bool(false),
		IamInstanceProfile: &ec2.IamInstanceProfileSpecification{
			Name: aws.String(os.Getenv("EC2_IAM_ROLE")),
		},
		TagSpecifications: []*ec2.TagSpecification{{
			Tags: []*ec2.Tag{{
				Key:   aws.String("Name"),
				Value: aws.String(os.Getenv("EC2_NAME_TAG")),
			}},
		}},
		InstanceInitiatedShutdownBehavior: aws.String("terminate"),
	}
	ec2KeyPair := os.Getenv("EC2_KEY_PAIR")
	if ec2KeyPair != "" {
		params.KeyName = aws.String(ec2KeyPair)
	}
	reservation, err := l.ec2.RunInstances(params)
	if err != nil {
		panic(err)
	}
	l.ip = *reservation.Instances[0].PublicIpAddress
}

func (l *dispatcher) checkIfAlreadyRunning() {
	params := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{{
			Name:   aws.String("tag:Name"),
			Values: []*string{aws.String(os.Getenv("EC2_NAME_TAG"))},
		}, {
			Name:   aws.String("instance-state-name"),
			Values: []*string{aws.String("running")},
		}},
	}
	resp, err := l.ec2.DescribeInstances(params)
	if err != nil {
		panic(err)
	}
	for _, reservation := range resp.Reservations {
		if len(reservation.Instances) > 0 {
			panic(ErrAlreadyRunning)
		}
	}
}

func (l *dispatcher) updateDnsRecord() {
	zoneId := os.Getenv("ROUTE53_ZONE_ID")
	if zoneId == "" {
		return
	}
	params := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{{
				Action: aws.String("UPSERT"),
				ResourceRecordSet: &route53.ResourceRecordSet{
					Name: aws.String(os.Getenv("ROUTE53_FQDN")),
					ResourceRecords: []*route53.ResourceRecord{{
						Value: aws.String(l.ip),
					}},
					TTL:  aws.Int64(60),
					Type: aws.String("A"),
				},
			}},
			Comment: aws.String("Update A record for Factorio server"),
		},
		HostedZoneId: aws.String(zoneId),
	}
	_, err := l.r53.ChangeResourceRecordSets(params)
	if err != nil {
		panic(err)
	}
}

func (l *dispatcher) UploadToS3(name string, file io.ReadSeeker) error {
	if l.s3folder.Path != "" {
		name = l.s3folder.Path + "/" + name
	}
	params := &s3.PutObjectInput{
		Bucket: aws.String(l.s3folder.Host),
		Key:    aws.String(name),
		Body:   file,
	}
	_, err := l.s3.PutObject(params)
	return err
}
