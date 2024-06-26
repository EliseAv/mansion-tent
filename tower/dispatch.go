package tower

import (
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/joho/godotenv"
)

type dispatcher struct {
	ec2      *ec2.EC2
	r53      *route53.Route53
	s3       *s3.S3
	s3folder url.URL
	userdata *string
	trying   sync.Mutex
	err      error
	instance *string
	ip       *string
}

var (
	ErrAlreadyRunning   = errors.New("instance already running")
	ErrNoAMI            = errors.New("no AMI found")
	ErrNoSecurityGroup  = errors.New("no security group found")
	ErrInstanceNotFound = errors.New("instance not found")
	ErrInstanceHasNoIP  = errors.New("instance has no IP")
)

func RunDispatcher() {
	NewDispatcher().ConsoleLaunch()
}

func NewDispatcher() *dispatcher {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(os.Getenv("AWS_REGION"))},
	}))
	sess3 := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String(os.Getenv("AWS_REGION_S3"))},
	}))
	sessUsE1 := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
		Config:            aws.Config{Region: aws.String("us-east-1")},
	}))
	l := &dispatcher{
		ec2: ec2.New(sess),
		r53: route53.New(sessUsE1),
		s3:  s3.New(sess3),
	}

	parsed, err := url.Parse(os.Getenv("S3_FOLDER_URL"))
	if err != nil {
		panic(err)
	}
	parsed.Path = strings.Trim(parsed.Path, "/")
	l.s3folder = *parsed
	l.s3folder.Path = strings.Trim(l.s3folder.Path, "/")
	l.userdata = l.generateUserData()

	l.uploadExecutable()
	return l
}

func (l *dispatcher) ConsoleLaunch() {
	l.LaunchFactorio()
	if l.err != nil {
		slog.Error("Launcher error", "err", l.err)
	} else {
		slog.Info("Launched instance", "hostname", os.Getenv("ROUTE53_FQDN"), "ip", *l.ip)
	}
}

func (l *dispatcher) uploadExecutable() {
	executable, err := os.Executable()
	if err != nil {
		panic(err)
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		// change the extension to .x64
		extension := filepath.Ext(executable)
		pos := len(executable) - len(extension)
		executable = executable[:pos] + ".x64"
	}
	file, err := os.Open(executable)
	if err != nil {
		panic(err)
	}
	err = l.UploadToS3("mt.x64", file)
	if err != nil {
		panic(err)
	}
	slog.Info("Uploaded", "file", file.Name())
}

func (l *dispatcher) LaunchFactorio() {
	defer func() {
		if recover() != nil {
			debug.PrintStack()
		}
	}()
	if !l.trying.TryLock() {
		panic(ErrAlreadyRunning)
	}
	l.checkIfAlreadyRunning()
	l.createInstance()
	l.updateDnsRecord()
	l.trying.Unlock()
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
	slog.Debug("Latest AMI",
		"id", *latestAmi.ImageId,
		"name", *latestAmi.Name,
		"date", *latestAmi.CreationDate)
	return latestAmi
}

func (l *dispatcher) generateUserData() *string {
	url := os.Getenv("S3_FOLDER_URL")
	values, err := godotenv.Read("mt.env")
	if err != nil {
		panic(err)
	}
	marshalled, err := godotenv.Marshal(values)
	if err != nil {
		panic(err)
	}
	lines := "#!/bin/bash\n" +
		"mkdir -p /opt/mansionTent\n" +
		"cat > /opt/mansionTent/mt.env <<EOF\n" +
		marshalled + "\nEOF\n" +
		"aws s3 cp " + url + "/mt.x64 /opt/mansionTent/mt.x64\n" +
		"chmod +x /opt/mansionTent/mt.x64\n" +
		"sudo -iu ec2-user screen -dm /opt/mansionTent/mt.x64 launch\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(lines))
	return aws.String(encoded)
}

func (l *dispatcher) generateTagSpecifications() []*ec2.TagSpecification {
	tags := []*ec2.Tag{{
		Key:   aws.String("Name"),
		Value: aws.String(os.Getenv("EC2_NAME_TAG")),
	}}
	return []*ec2.TagSpecification{
		{ResourceType: aws.String("instance"), Tags: tags},
		{ResourceType: aws.String("volume"), Tags: tags},
	}
}

func (l *dispatcher) createInstance() {
	params := &ec2.RunInstancesInput{
		ImageId:      l.getLatestAmazonLinuxAMI().ImageId,
		InstanceType: aws.String(os.Getenv("EC2_INSTANCE_TYPE")),
		MinCount:     aws.Int64(1),
		MaxCount:     aws.Int64(1),
		UserData:     l.userdata,
		DryRun:       aws.Bool(false),
		IamInstanceProfile: &ec2.IamInstanceProfileSpecification{
			Name: aws.String(os.Getenv("EC2_IAM_ROLE")),
		},
		TagSpecifications:                 l.generateTagSpecifications(),
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
	instance := reservation.Instances[0]
	slog.Debug("Launched",
		"instance", *instance.InstanceId,
		"ami", *instance.ImageId,
		"type", *instance.InstanceType,
		"key", *instance.KeyName,
		"state", *instance.State.Name)
	l.instance = instance.InstanceId
	l.ip = l.checkForIp()
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

func (l *dispatcher) checkForIp() *string {
	describe := &ec2.DescribeInstancesInput{
		InstanceIds: []*string{l.instance},
	}
	err := l.ec2.WaitUntilInstanceRunning(describe)
	if err != nil {
		panic(err)
	}
	slog.Debug("Instance is running", "id", *l.instance)
	description, err := l.ec2.DescribeInstances(describe)
	if err != nil {
		panic(err)
	}
	if len(description.Reservations) == 0 || len(description.Reservations[0].Instances) == 0 {
		panic(ErrInstanceNotFound)
	}
	instance := description.Reservations[0].Instances[0]
	if instance.PublicIpAddress != nil {
		return instance.PublicIpAddress
	}
	panic(ErrInstanceHasNoIP)
}

func (l *dispatcher) updateDnsRecord() {
	zoneId := os.Getenv("ROUTE53_ZONE_ID")
	if zoneId == "" || l.ip == nil {
		return
	}
	params := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{{
				Action: aws.String("UPSERT"),
				ResourceRecordSet: &route53.ResourceRecordSet{
					Name: aws.String(os.Getenv("ROUTE53_FQDN")),
					ResourceRecords: []*route53.ResourceRecord{{
						Value: l.ip,
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
