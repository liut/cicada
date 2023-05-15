# Cicada

A dns server for CI/CD


## Getting started

```bash
sudo apt install redis-server

# build
make dist

# usage
./cicada -h

# add a record
./cicada -name feature-dev.mycom.work -ip 172.18.19.3 -days 10

# start dns server
./cicada -serv -port=1153


# test
dig @127.0.0.1 -p 1153 feature-dev.mycom.work


# update with nsupdate
echo "server 127.0.0.1 1153
zone mycom.work.
update add feature-dev.mycom.work. 180 IN A 172.18.19.4
send
quit
" | nsupdate -d


```
