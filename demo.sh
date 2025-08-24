curl -s http://localhost:8081/healthz | jq .

curl -s http://localhost:8081/order/b563feb7b2b84b6test | jq .

curl -s -o /dev/null -w "t=%{time_total}\n" http://localhost:8081/order/b563feb7b2b84b6test

b563feb7b2b84b6test
./scripts/produce.sh fixtures/model.json DIFF_KEY
