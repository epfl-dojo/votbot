main: get_dep
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

get_dep:
	go get github.com/go-telegram-bot-api/telegram-bot-api 
	go get gopkg.in/yaml.v2

docker: main
	cp /etc/ssl/certs/ca-certificates.crt .
	docker build . -t votbot

clean:
	rm -f ca-certificates main votbot
