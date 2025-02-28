# This Fork is used to develop a simulation environment for G-SINC and is still WIP

# Secure and Dependable Clock Synchronization on Next-Generation Networks

Secure and dependable clock synchronization is an essential prerequisite for many industries with applications in finance, telecommunication, electric power production and distribution, or environmental monitoring.

Current best practice to achieve large-scale clock synchronization relies on global navigation satellite systems (GNSSes) at the considerable risk of being exposed to outages, malfunction, or attacks against availability and accuracy. Natural disasters like solar superstorms also have the potential to hit and severely impact GNSSes.

It is therefore all too apparent that clock synchronization solely based on GNSSes as global reference clocks does not fulfill fundamental dependability requirements for systems that serve indispensable functionalities in our society. Facing these concerns, governments have issued mandates to protect critical infrastructure services from disruption to GNSS services, including a 2020 US Executive Order. Operators and equipment manufacturers are encouraged to intensify research and development of alternative technologies in this space.

Aiming to join these efforts, we are proposing G-SINC: a novel global, Byzantine fault-tolerant clock synchronization approach that does not place trust in any single entity and is able to tolerate a fraction of faulty entities while still maintaining accurate synchronization on a global scale among otherwise sovereign network topologies. G-SINC can be implemented as a fully backward compatible active standby solution for existing time synchronization deployments.

![G-SINC architecture overview](/doc/overview.png)

This is achieved by building on the solid body of fault-tolerant clock synchronization research dating all the way back to the 1980s and the SCION Internet architecture providing required resilience and security properties at the network level as an intrinsic consequence of its underlying design principles. Besides the possibility to use multiple distinct network paths in parallel for significantly improved fault-tolerance, we highlight the fact that SCION paths are reversible and therefore symmetric. Hence, they help to increase time synchronization precision compared to clock offset measurements over the often asymmetric paths in today’s Internet.

We are currently building out the first end-to-end implementation of G-SINC which is contained in this repository.


## Publication

G-SINC: Global Synchronization Infrastructure for Network Clocks.
Marc Frei, Jonghoon Kwon, Seyedali Tabaeiaghdaei, Marc Wyss, Christoph Lenzen, and Adrian Perrig.
In Proceedings of the Symposium on Reliable Distributed Systems (SRDS) 2022.
\[[pdf](https://netsec.ethz.ch/publications/papers/G-SINC.pdf)\], \[[doi](https://doi.org/10.1109/SRDS55811.2022.00021)\], \[[arXiv](https://arxiv.org/abs/2207.06116)\]


## Running a simple IP-based server

Reference platform: Ubuntu 22.04 LTS, Go 1.21; see [below](https://github.com/marcfrei/scion-time/edit/main/README.md#installing-prerequisites-for-a-scion-test-environment).

```bash
cd ~
git clone https://github.com/marcfrei/scion-time.git

echo >> .bash_profile
echo 'export SCION_TIME_ROOT=~/scion-time' >> ~/.bash_profile # Change this path if neccessary
source ~/.bash_profile

cd $SCION_TIME_ROOT
go build timeservice.go timeservicex.go

sudo $SCION_TIME_ROOT/timeservice server -verbose -config testnet/test-server.toml
```

## Querying an IP-based server

In an additional session:

```bash
$SCION_TIME_ROOT/timeservice tool -verbose -local 0-0,0.0.0.0 -remote 0-0,127.0.0.1:123
```

## Querying an IP-based server with Network Time Security (NTS)

In an additional session:

```bash
$SCION_TIME_ROOT/timeservice tool -verbose -local 0-0,0.0.0.0 -remote 0-0,127.0.0.1:4460 -auth nts -ntske-insecure-skip-verify
```

## Installing prerequisites for a SCION test environment

Reference platform: Ubuntu 22.04 LTS, Go 1.21.2

```bash
sudo apt-get update
sudo apt-get install -y build-essential python3-pip supervisor
pip3 install plumbum toml supervisor-wildcards
```

On x86-64:

```bash
sudo rm -rf /usr/local/go
curl -LO https://golang.org/dl/go1.21.6.linux-amd64.tar.gz
echo "3f934f40ac360b9c01f616a9aa1796d227d8b0328bf64cb045c7b8c4ee9caea4 go1.21.6.linux-amd64.tar.gz" | sha256sum -c
sudo tar -C /usr/local -xzf go1.21.6.linux-amd64.tar.gz
rm go1.21.6.linux-amd64.tar.gz
echo >> .bash_profile
echo 'export PATH=$PATH:/usr/local/go/bin' >> .bash_profile
source ~/.bash_profile
go version
```

On ARM64:

```bash
sudo rm -rf /usr/local/go
curl -LO https://golang.org/dl/go1.21.6.linux-arm64.tar.gz
echo "e2e8aa88e1b5170a0d495d7d9c766af2b2b6c6925a8f8956d834ad6b4cacbd9a go1.21.6.linux-arm64.tar.gz" | sha256sum -c
sudo tar -C /usr/local -xzf go1.21.6.linux-arm64.tar.gz
rm go1.21.6.linux-arm64.tar.gz
echo >> .bash_profile
echo 'export PATH=$PATH:/usr/local/go/bin' >> .bash_profile
source ~/.bash_profile
go version
```

## Setting up a SCION test environment

```bash
cd ~
git clone https://github.com/scionproto/scion.git

echo >> .bash_profile
echo 'export SCION_ROOT=~/scion' >> ~/.bash_profile # Change this path if neccessary
source ~/.bash_profile

cd $SCION_ROOT
git checkout v0.10.0
go build -o ./bin/ ./control/cmd/control
go build -o ./bin/ ./daemon/cmd/daemon
go build -o ./bin/ ./dispatcher/cmd/dispatcher
go build -o ./bin/ ./router/cmd/router
go build -o ./bin/ ./scion/cmd/scion
go build -o ./bin/ ./scion-pki/cmd/scion-pki

cd ~
git clone https://github.com/marcfrei/scion-time.git

echo >> .bash_profile
echo 'export SCION_TIME_ROOT=~/scion-time' >> ~/.bash_profile # Change this path if neccessary
source ~/.bash_profile

cd $SCION_TIME_ROOT
go build timeservice.go timeservicex.go
```

## Starting the SCION test network

```bash
cd $SCION_TIME_ROOT/testnet

rm -rf logs
./scion-topo-gen-crypto.sh
./testnet-up.sh

./supervisor/supervisor.sh reload && sleep 1
./supervisor/supervisor.sh start all
```

## Running the servers

In session no. 1, run server at `1-ff00:0:111,10.1.1.11:10123`:

```bash
cd $SCION_TIME_ROOT
sudo ip netns exec netns0 ./timeservice server -verbose -config testnet/gen-eh/ASff00_0_111/ts1-ff00_0_111-1.toml
```

In session no. 2, run server at `1-ff00:0:112,10.1.1.12:10123`:

```bash
cd $SCION_TIME_ROOT
sudo ip netns exec netns1 ./timeservice server -verbose -config testnet/gen-eh/ASff00_0_112/ts1-ff00_0_112-1.toml
```

## Querying SCION-based servers

In an additional session, query server at `1-ff00:0:111,10.1.1.11:10123` from `1-ff00:0:112,10.1.1.12`:

```bash
sudo ip netns exec netns1 $SCION_TIME_ROOT/timeservice tool -verbose -daemon 10.1.1.12:30255 -local 1-ff00:0:112,10.1.1.12 -remote 1-ff00:0:111,10.1.1.11:10123
```

Or query server at `1-ff00:0:112,10.1.1.12:10123` from `1-ff00:0:111,10.1.1.11`:

```bash
sudo ip netns exec netns0 $SCION_TIME_ROOT/timeservice tool -verbose -daemon 10.1.1.11:30255 -local 1-ff00:0:111,10.1.1.11 -remote 1-ff00:0:112,10.1.1.12:10123
```

### Querying a SCION-based server with SCION Packet Authenticator Option (SPAO)

```bash
sudo ip netns exec netns1 $SCION_TIME_ROOT/timeservice tool -verbose -daemon 10.1.1.12:30255 -local 1-ff00:0:112,10.1.1.12 -remote 1-ff00:0:111,10.1.1.11:10123 -auth spao
```

### Querying a SCION-based server with Network Time Security (NTS)

```bash
sudo ip netns exec netns1 $SCION_TIME_ROOT/timeservice tool -verbose -daemon 10.1.1.12:30255 -local 1-ff00:0:112,10.1.1.12 -remote 1-ff00:0:111,10.1.1.11:14460 -auth nts -ntske-insecure-skip-verify
```

### Querying a SCION-based server with SPAO and NTS

```bash
sudo ip netns exec netns1 $SCION_TIME_ROOT/timeservice tool -verbose -daemon 10.1.1.12:30255 -local 1-ff00:0:112,10.1.1.12 -remote 1-ff00:0:111,10.1.1.11:14460 -auth spao,nts -ntske-insecure-skip-verify
```

### Querying a SCION-based server via IP

```bash
sudo ip netns exec netns1 $SCION_TIME_ROOT/timeservice tool -verbose -local 0-0,10.1.1.12 -remote 0-0,10.1.1.11:123
```

### Querying a SCION-based server via IP with NTS

```bash
sudo ip netns exec netns1 $SCION_TIME_ROOT/timeservice tool -verbose -local 0-0,10.1.1.12 -remote 0-0,10.1.1.11:4460 -auth nts -ntske-insecure-skip-verify
```

## Synchronizing with a SCION-based server

In session no. 1, run server at `1-ff00:0:111,10.1.1.11:10123`:

```bash
cd $SCION_TIME_ROOT
sudo ip netns exec netns0 ./timeservice server -verbose -config testnet/gen-eh/ASff00_0_111/ts1-ff00_0_111-1.toml
```

And in session no. 2, synchronize node `1-ff00:0:112,10.1.1.12` with server at `1-ff00:0:111,10.1.1.11:10123`:

```bash
cd $SCION_TIME_ROOT
sudo ip netns exec netns1 ./timeservice client -verbose -config testnet/gen-eh/ASff00_0_112/test-client.toml
```

## Stopping the SCION test network

```bash
export SCION_BIN=$SCION_ROOT/bin
cd $SCION_TIME_ROOT/testnet

./supervisor/supervisor.sh stop all
```
