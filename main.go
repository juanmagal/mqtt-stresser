package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"
)

var (
	resultChan         = make(chan Result)
	stopWaitLoop       = false
	tearDownInProgress = false
	randomSource       = rand.New(rand.NewSource(time.Now().UnixNano()))

	subscriberClientIdTemplate = "mqtt-stresser-sub-%s-worker%d-%d"
	publisherClientIdTemplate  = "mqtt-stresser-pub-%s-worker%d-%d"
	topicNameTemplate          = "internal/mqtt-stresser/%s/worker%d-%d"

	errorLogger   = log.New(os.Stderr, "ERROR: ", log.Lmicroseconds|log.Ltime|log.Lshortfile)
	verboseLogger = log.New(os.Stderr, "DEBUG: ", log.Lmicroseconds|log.Ltime|log.Lshortfile)

	argNumClients          = flag.Int("num-clients", 10, "Number of concurrent clients")
	argNumMessages         = flag.Int("num-messages", 10, "Number of messages shipped by client")
	argTimeout             = flag.String("timeout", "5s", "Timeout for pub/sub actions")
	argGlobalTimeout       = flag.String("global-timeout", "60s", "Timeout spanning all operations")
	argRampUpSize          = flag.Int("rampup-size", 100, "Size of rampup batch. Default rampup batch size is 100.")
	argRampUpDelay         = flag.String("rampup-delay", "500ms", "Time between batch rampups")
	argBrokerUrl           = flag.String("broker", "", "Broker URL")
	argUsername            = flag.String("username", "", "Username")
	argPassword            = flag.String("password", "", "Password")
	argLogLevel            = flag.Int("log-level", 0, "Log level (0=nothing, 1=errors, 2=debug, 3=error+debug)")
	argProfileCpu          = flag.String("profile-cpu", "", "write cpu profile `file`")
	argProfileMem          = flag.String("profile-mem", "", "write memory profile to `file`")
	argHideProgress        = flag.Bool("no-progress", false, "Hide progress indicator")
	argHelp                = flag.Bool("help", false, "Show help")
	argRetain              = flag.Bool("retain", false, "if set, the retained flag of the published mqtt messages is set")
	argPublisherQoS        = flag.Int("publisher-qos", 0, "QoS level of published messages")
	argSubscriberQoS       = flag.Int("subscriber-qos", 0, " QoS level for the subscriber")
	argSkipTLSVerification = flag.Bool("skip-tls-verification", false, "skip the tls verfication of the MQTT Connection")
        argTopic               = flag.String("topic", "", "Topic")
        argSenMlFormat         = flag.Bool("senml", false, "Messages sent in SenML format")
        argSenMlDeviceName     = flag.String("senmldevicename","device","Device name")
        argSenMlMeasure        = flag.String("senmlmeasure","temperature","Measure used (e.g. temperature)")
)

type Result struct {
	WorkerId          int
	Event             string
	PublishTime       time.Duration
	ReceiveTime       time.Duration
	MessagesReceived  int
	MessagesPublished int
	Error             bool
	ErrorMessage      error
}

type TimeoutError interface {
	Timeout() bool
	Error() string
}

func parseQosLevels(qos int, role string) (byte, error) {
	if qos < 0 || qos > 2 {
		return 0, fmt.Errorf("%d is an invalid QoS level for %s. Valid levels are 0, 1 and 2", qos, role)
	}
	return byte(qos), nil
}

func main() {
	flag.Parse()

	if flag.NFlag() < 1 || *argHelp {
		flag.Usage()
		if *argHelp {
			os.Exit(0)
		}
		os.Exit(1)
	}

	if *argProfileCpu != "" {
		f, err := os.Create(*argProfileCpu)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not create CPU profile: %s\n", err)
			os.Exit(1)
		}

		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Fprintf(os.Stderr, "Could not start CPU profile: %s\n", err)
			os.Exit(1)
		}
	}

	num := *argNumMessages
	username := *argUsername
	password := *argPassword
        topic := *argTopic
	actionTimeout, err := time.ParseDuration(*argTimeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not parse '--timeout': '%s' is not a valid duration string. See https://golang.org/pkg/time/#ParseDuration for valid duration strings\n", *argGlobalTimeout)
		os.Exit(1)
	}

	verboseLogger.SetOutput(ioutil.Discard)
	errorLogger.SetOutput(ioutil.Discard)

	if *argLogLevel == 1 || *argLogLevel == 3 {
		errorLogger.SetOutput(os.Stderr)
	}

	if *argLogLevel == 2 || *argLogLevel == 3 {
		verboseLogger.SetOutput(os.Stderr)
	}

	if *argBrokerUrl == "" {
		fmt.Fprintln(os.Stderr, "'--broker' is empty. Abort.")
		os.Exit(1)
	}

	var publisherQoS, subscriberQoS byte

	if lvl, err := parseQosLevels(*argPublisherQoS, "publisher"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	} else {
		publisherQoS = lvl
	}

	if lvl, err := parseQosLevels(*argSubscriberQoS, "subscriber"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	} else {
		subscriberQoS = lvl
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	rampUpDelay, _ := time.ParseDuration(*argRampUpDelay)
	rampUpSize := *argRampUpSize

	if rampUpSize < 0 {
		rampUpSize = 100
	}

	resultChan = make(chan Result, *argNumClients**argNumMessages)

	globalTimeout, err := time.ParseDuration(*argGlobalTimeout)
	if err != nil {
		fmt.Printf("Could not parse '--global-timeout': '%s' is not a valid duration string. See https://golang.org/pkg/time/#ParseDuration for valid duration strings\n", *argGlobalTimeout)
		os.Exit(1)
	}
	testCtx, cancelFunc := context.WithTimeout(context.Background(), globalTimeout)

	stopStartLoop := false
	for cid := 0; cid < *argNumClients && !stopStartLoop; cid++ {

		if cid%rampUpSize == 0 && cid > 0 {
			fmt.Printf("%d worker started - waiting %s\n", cid, rampUpDelay)
			time.Sleep(rampUpDelay)
			select {
			case <-time.NewTimer(rampUpDelay).C:
			case s := <-signalChan:
				fmt.Printf("Got signal %s. Cancel test.\n", s.String())
				cancelFunc()
				stopStartLoop = true
			}
		}

		go (&Worker{
			WorkerId:            cid,
			BrokerUrl:           *argBrokerUrl,
			Username:            username,
			Password:            password,
                        Topic:               topic,
			SkipTLSVerification: *argSkipTLSVerification,
			NumberOfMessages:    num,
			Timeout:             actionTimeout,
			Retained:            *argRetain,
			PublisherQoS:        publisherQoS,
			SubscriberQoS:       subscriberQoS,
                        FormatWithSenMl:     *argSenMlFormat,
                        DeviceName:          *argSenMlDeviceName,
                        Measure:             *argSenMlMeasure,
		}).Run(testCtx)
	}
	fmt.Printf("%d worker started\n", *argNumClients)

	finEvents := 0

	results := make([]Result, *argNumClients)

	for finEvents < *argNumClients && !stopWaitLoop {
		select {
		case msg := <-resultChan:
			results[msg.WorkerId] = msg

			if msg.Event == CompletedEvent || msg.Error {
				finEvents++
				verboseLogger.Printf("%d/%d events received\n", finEvents, *argNumClients)
			}

			if msg.Error {
				errorLogger.Println(msg)
			}

			if *argHideProgress == false {
				if msg.Event == CompletedEvent {
					fmt.Print(".")
				}

				if msg.Error {
					fmt.Print("E")
				}
			}

		case <-testCtx.Done():
			switch testCtx.Err().(type) {
			case TimeoutError:
				fmt.Println("Test timeout. Wait 5s to allow disconnection of clients.")
			default:
				fmt.Println("Test canceled. Wait 5s to allow disconnection of clients.")
			}
			time.Sleep(5 * time.Second)
			stopWaitLoop = true
		case s := <-signalChan:
			fmt.Printf("Got signal %s. Cancel test.\n", s.String())
			cancelFunc()
			stopWaitLoop = true
		}
	}

	summary, err := buildSummary(*argNumClients, num, results)
	exitCode := 0

	if err != nil {
		exitCode = 1
	} else {
		printSummary(summary)
	}

	if *argProfileMem != "" {
		f, err := os.Create(*argProfileMem)

		if err != nil {
			fmt.Printf("Could not create memory profile: %s\n", err)
		}

		runtime.GC() // get up-to-date statistics

		if err := pprof.WriteHeapProfile(f); err != nil {
			fmt.Printf("Could not write memory profile: %s\n", err)
		}
		f.Close()
	}

	pprof.StopCPUProfile()

	os.Exit(exitCode)
}
