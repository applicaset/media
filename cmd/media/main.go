package main

import (
	"github.com/applicaset/media/app"
	"github.com/applicaset/pkg/serve"
)

func main() { serve.Main(app.Run) }
