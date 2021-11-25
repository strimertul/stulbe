# stulbe - strimertul backend

Back-end portion of strimertul for features that require one:

- KV sync for remote applications
- User-facing loyalty points info (redeem APIs not available yet sorry!)
- Webhook subscription for alerts

Platform support is limited to Twitch only for the time being (sorry!)

## Getting started

### Docker image

A prebuilt docker image is available in Docker Hub at [strimertul/stulbe](https://hub.docker.com/r/strimertul/stulbe).

### Pre-build binaries

You can find pre-build binaries for Windows and Linux in our [Release section](https://github.com/strimertul/stulbe/releases/latest).

### Building manually

You can build the app like any other Go project:

```sh
go build
```

### Starting stulbe

To start stulbe succesfully you will have to configure the following environment variables:

```env
TWITCH_CLIENT_ID=Twitch client ID
TWITCH_CLIENT_SECRET=Twitch client secret
TWITCH_WEBHOOK_SECRET=some random secret string
REDIRECT_URI=https://redirect.uri.for.auth/oauth
WEBHOOK_URI=https://webhook.uri.for.twitch.alerts/webhook
```

To obtain the Twitch client credentials, [create an Application in the Twitch dev console](https://dev.twitch.tv/console/apps/create), make sure to set the REDIRECT_URI to a reacheable URL and to make sure it's in the "OAuth Redirect URLs" section of the application!

## License

The entire project is licensed under [AGPL-3.0-only](LICENSE) (see `LICENSE`).

## FAQ

### How do I use it?

Swagger or whatever docs coming soon, meanwhile look at [this Go client](https://github.com/strimertul/stulbe-client-go).

### Does this scale?

lol no it uses a single-writer on-disk KV store

### Make it scale then!

The aim of the strimertul suite is to be lean and hackable. Making a distributed cloud-native _\<more devop buzzwords here>_ thingamajig is way out of scope.

I don't know realistically how much load this system can take, but I highly suggest looking elsewhere if you think your scale could be an issue. If you made/know a FOSS tool like this that scales, let me know!

### Where's the API docs?

Soon™

### Gib awoos

[Here you go](https://youtu.be/pKcR7qHlAIA?list=LL&t=75)
