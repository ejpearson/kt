package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"strings"

	"github.com/Shopify/sarama"
)

type adminCmd struct {
	brokers    []string
	verbose    bool
	version    sarama.KafkaVersion
	tlsCA      string
	tlsCert    string
	tlsCertKey string

	createTopic  string
	topicDetail  *sarama.TopicDetail
	validateOnly bool
	deleteTopic  string

	admin sarama.ClusterAdmin
}

type adminArgs struct {
	brokers    string
	verbose    bool
	version    string
	tlsCA      string
	tlsCert    string
	tlsCertKey string

	createTopic     string
	topicDetailPath string
	validateOnly    bool
	deleteTopic     string
}

func (cmd *adminCmd) parseArgs(as []string) {
	var (
		args = cmd.parseFlags(as)
	)

	cmd.verbose = args.verbose
	cmd.version = kafkaVersion(args.version)
	cmd.tlsCA = args.tlsCA
	cmd.tlsCert = args.tlsCert
	cmd.tlsCertKey = args.tlsCertKey

	envBrokers := os.Getenv("KT_BROKERS")
	if args.brokers == "" {
		if envBrokers != "" {
			args.brokers = envBrokers
		} else {
			args.brokers = "localhost:9092"
		}
	}
	cmd.brokers = strings.Split(args.brokers, ",")
	for i, b := range cmd.brokers {
		if !strings.Contains(b, ":") {
			cmd.brokers[i] = b + ":9092"
		}
	}

	cmd.validateOnly = args.validateOnly
	cmd.createTopic = args.createTopic
	cmd.deleteTopic = args.deleteTopic

	if cmd.createTopic != "" {
		buf, err := ioutil.ReadFile(args.topicDetailPath)
		if err != nil {
			failf("failed to read topic detail err=%v", err)
		}

		var detail sarama.TopicDetail
		if err = json.Unmarshal(buf, &detail); err != nil {
			failf("failed to unmarshal topic detail err=%v", err)
		}
		cmd.topicDetail = &detail
	}
}

func (cmd *adminCmd) run(args []string) {
	var err error

	cmd.parseArgs(args)

	if cmd.verbose {
		sarama.Logger = log.New(os.Stderr, "", log.LstdFlags)
	}

	if cmd.admin, err = sarama.NewClusterAdmin(cmd.brokers, cmd.saramaConfig()); err != nil {
		failf("failed to create cluster admin err=%v", err)
	}

	if cmd.createTopic != "" {
		cmd.runCreateTopic()

	} else if cmd.deleteTopic != "" {
		cmd.runDeleteTopic()
	} else {
		failf("need to supply at least one sub-command of: createtopic, deletetopic")
	}
}

func (cmd *adminCmd) runCreateTopic() {
	err := cmd.admin.CreateTopic(cmd.createTopic, cmd.topicDetail, cmd.validateOnly)
	if err != nil {
		failf("failed to create topic err=%v", err)
	}
}

func (cmd *adminCmd) runDeleteTopic() {
	err := cmd.admin.DeleteTopic(cmd.deleteTopic)
	if err != nil {
		failf("failed to delete topic err=%v", err)
	}
}

func (cmd *adminCmd) saramaConfig() *sarama.Config {
	var (
		err error
		usr *user.User
		cfg = sarama.NewConfig()
	)

	cfg.Version = cmd.version
	if usr, err = user.Current(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read current user err=%v", err)
	}
	cfg.ClientID = "kt-admin-" + sanitizeUsername(usr.Username)

	tlsConfig, err := setupCerts(cmd.tlsCert, cmd.tlsCA, cmd.tlsCertKey)
	if err != nil {
		failf("failed to setup certificates err=%v", err)
	}
	if tlsConfig != nil {
		cfg.Net.TLS.Enable = true
		cfg.Net.TLS.Config = tlsConfig
	}

	return cfg
}

func (cmd *adminCmd) parseFlags(as []string) adminArgs {
	var args adminArgs
	flags := flag.NewFlagSet("consume", flag.ContinueOnError)
	flags.StringVar(&args.brokers, "brokers", "", "Comma separated list of brokers. Port defaults to 9092 when omitted (defaults to localhost:9092).")
	flags.BoolVar(&args.verbose, "verbose", false, "More verbose logging to stderr.")
	flags.StringVar(&args.version, "version", "", "Kafka protocol version")
	flags.StringVar(&args.tlsCA, "tlsca", "", "Path to the TLS certificate authority file")
	flags.StringVar(&args.tlsCert, "tlscert", "", "Path to the TLS client certificate file")
	flags.StringVar(&args.tlsCertKey, "tlscertkey", "", "Path to the TLS client certificate key file")

	flags.StringVar(&args.createTopic, "createtopic", "", "Name of the topic that should be created.")
	flags.StringVar(&args.topicDetailPath, "topicdetail", "", "Path to JSON encoded topic detail. cf sarama.TopicDetail")
	flags.BoolVar(&args.validateOnly, "validateonly", false, "Flag to indicate whether operation should only validate input (supported for createtopic).")

	flags.StringVar(&args.deleteTopic, "deletetopic", "", "Name of the topic that should be deleted.")

	flags.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage of admin:")
		flags.PrintDefaults()
		fmt.Fprintln(os.Stderr, adminDocString)
	}

	err := flags.Parse(as)
	if err != nil && strings.Contains(err.Error(), "flag: help requested") {
		os.Exit(0)
	} else if err != nil {
		os.Exit(2)
	}

	return args
}

var adminDocString = `
The value for -brokers can also be set via environment variables KT_BROKERS.
The value supplied on the command line wins over the environment variable value.

If both -createtopic and deletetopic are supplied, -createtopic wins.

The topic details should be passed via a JSON file that represents a sarama.TopicDetail struct.
cf https://godoc.org/github.com/Shopify/sarama#TopicDetail

A simple way to pass a JSON file is to use a tool like https://github.com/fgeller/jsonify and shell's process substition:

kt admin -createtopic morenews -topicdetail <(jsonify =NumPartitions 1 =ReplicationFactor 1)`
