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
./cicada -serv -port=1353


# test
dig @127.0.0.1 -p 1353 feature-dev.mycom.work


# update with nsupdate
echo "server 127.0.0.1 1353
zone mycom.work.
update add feature-dev.mycom.work. 180 IN A 172.18.19.4
send
quit
" | nsupdate -d


# update with http PUT
curl -X PUT -H "Content-Type: application/json" -d '[{"name":"feature-dev.mycom.work","ip":"172.18.19.5"}]' http://localhost:1354/api/dns/a

```
