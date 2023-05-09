# CIDNS

A dns server for CI/CD


## Getting started

```bash
sudo apt install redis-server

# build
make dist

# usage
./cidns -h

# add a record
./cidns -name feature-dev.mycom.work -ip 172.18.19.3 -days 10

# start dns server
./cidns -serv -port=1153


# test
dig @127.0.0.1 -p 1153 feature-dev.mycom.work

```
