FROM scratch
ADD static /static
ADD controller /controller
EXPOSE 8080
ENTRYPOINT ["/controller"]
#sudo docker build -t cjt/shipyard .
#sudo docker run -p 8081:8080 --name cjt_shipyard --link shipyard-rethinkdb:rethinkdb cjt/shipyard
