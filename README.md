# MQTT Stresser

Load testing tool to stress MQTT message broker

## Build

```
$ mkdir -p ${GOPATH}/src/github.com/inovex/
$ git clone https://github.com/inovex/mqtt-stresser.git ${GOPATH}/src/github.com/inovex/mqtt-stresser/
$ cd ${GOPATH}/src/github.com/inovex/mqtt-stresser/
$ make linux
```

This will build the mqtt stresser for linux target platform and write them to the ``build/`` directory.

If you want to build the Docker container version of this, go to repository directory and simply type ``docker build .``

## Install

Place the binary somewhere in a ``PATH`` directory and make it executable (``chmod +x mqtt-stresser``).

If you are using the container version, just type ``docker run flaviostutz/mqtt-stresser [options]`` for running mqtt-stresser.

## Configure

See ``mqtt-stresser -h`` for a list of available arguments.

## Run

Simple hello-world test using ``tcp://127.0.0.1:1883`` broker:

```
$./mqtt-stresser-linux-amd64 -broker tcp://127.0.0.1:1883 -num-clients 100 -num-messages 100 -username admin -password public -topic DataTopic -senml -rampup-delay 1s -rampup-size 10 -global-timeout 180s -timeout 20s
10 worker started - waiting 1s
20 worker started - waiting 1s
30 worker started - waiting 1s
40 worker started - waiting 1s
50 worker started - waiting 1s
60 worker started - waiting 1s
70 worker started - waiting 1s
80 worker started - waiting 1s
90 worker started - waiting 1s
100 worker started
....................................................................................................
# Configuration
Concurrent Clients: 100
Messages / Client:  10000

# Results
Published Messages: 10000 (100%)
Received Messages:  10000 (100%)
Completed:          100 (100%)
Errors:             0 (0%)

# Publishing Throughput
Fastest: 163738 msg/sec
Slowest: 5754 msg/sec
Median: 33656 msg/sec

  < 21552 msg/sec  38%
  < 37351 msg/sec  52%
  < 53149 msg/sec  64%
  < 68947 msg/sec  80%
  < 84746 msg/sec  88%
  < 100544 msg/sec  94%
  < 116343 msg/sec  98%
  < 132141 msg/sec  99%
  < 179536 msg/sec  100%

# Receiving Througput
Fastest: 283090 msg/sec
Slowest: 5424 msg/sec
Median: 23076 msg/sec

  < 33190 msg/sec  59%
  < 60957 msg/sec  68%
  < 88724 msg/sec  75%
  < 116490 msg/sec  79%
  < 144257 msg/sec  83%
  < 172024 msg/sec  94%
  < 199790 msg/sec  96%
  < 227557 msg/sec  97%
  < 283090 msg/sec  99%
  < 310857 msg/sec  100%
```
If using container, 
```
$ docker run inovex/mqtt-stresser -broker tcp://broker.mqttdashboard.com:1883 -num-clients 100 -num-messages 10 -rampup-delay 1s -rampup-size 10 -global-timeout 180s -timeout 20s
```
