# get_host

get_host is an DNS cache with HTTP REST API that enables easy lookups against part of hostname.

The primary goal, why I have written it, is to enable tab completion for ssh to hosts. This includes hosts that not yet have been connected to (In other words, hosts that is not saved in ~/.ssh/known_hosts).



## Build and run server 
```
go get -u github.com/spetzreborn/get_host
```
Use the normal go-toolchain to build the server
```
cd get_host/cmd/server
go build
```

Edit example.toml, current implementation requires permission to to an AXFR ([DNS Zone transfer](https://en.wikipedia.org/wiki/DNS_zone_transfer)) from the DNS-server.
Future version might have support for AXFR with TSIG and/or IXFR

Starting server
```
./server -configfile example.toml
```

## Run server as Docker
Copy and edit example.toml to ./config/config.toml
```
docker run -p 8080:8080/tcp -v $(pwd)/config:/config get_host
```

## Usage
### Verify and status
To verify that the server works:
```
curl -s localhost:8080/hosts/KEYWORD
```
To get information on server uptime, number of elements in cache, cache age and serial of SOA:
```
curl -s localhost:8080/status
```
### Use HTTP REST API
```
curl -s localhost:8080/hosts/partOfName
```
Results in:
```json
["partofname-server.example.tld", "partofname2-server.example.tld", "server-partofname.example.tld"]
```

### Use client (preferred)
This repository also includes an client that
1. First tries to connect to the configured server
2. If that don't work it tries to do an AXFR and match the KEYWORD itself

## Make ssh and tab completion work
### Alias of ssh
It is strongly recommended not to make an alias that overwrites  *ssh(1)*, but instead make an new alias or function that is used for sshing instead.
This is because if there is an bug or failure in this program or function there is always the possibility to fall back to "native" *ssh(1)*

Example of an bash function that handle ssh login and is suitable for tab completion [function.sh](function.sh)

### Bash completion

To make the function in [function.sh](function.sh) work with host completion it needs to be configured with bash completion.
Example of an bash function in [get_host-completion.bash](get_host-completion.bash)

### Source the completion file

Make sure that the bash [get_host-completion.bash](get_host-completion.bash) is sourced in .bashrc
```bash
. ~/get_host-completion.bash
```

