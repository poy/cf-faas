package main

import faas "github.com/poy/cf-faas"

func main() {
	faas.Start(faas.HandlerFunc(func(req faas.Request) (faas.Response, error) {
		return faas.Response{
			StatusCode: 200,
			Body:       req.Body,
		}, nil
	}))
}
