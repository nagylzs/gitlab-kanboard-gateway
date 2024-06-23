package webhooks

var PushQueue = make(chan PushEvent, 1000)
