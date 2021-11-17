# stulbe - strimertul backend

Back-end portion of strimertul for features that require one:

- KV sync for remote applications
- User-facing loyalty points info (redeem APIs not available yet sorry!)
- Webhook subscription for alerts

Platform support is limited to Twitch only for the time being (sorry!)

## Building

Just build the app like any other Go project

```sh
go build
```

## License

The entire project is licensed under [AGPL-3.0-only](LICENSE) (see `LICENSE`).

## FAQ

### How do I use it?

Swagger or whatever docs coming soon, meanwhile look at [this Go client](https://github.com/strimertul/stulbe-client-go).

### Does this scale?

lol no it uses a single-writer on-disk KV store

### Make it scale then!

The aim of the strimertul suite is to be lean and hackable. Making a distributed cloud-native _\<more devop buzzwords here>_ thingamajig is way out of scope.

If you have enough load you should consider going for Twitch Partner or making your own system (or fork us and make a stulbe-at-scale, FOSS for the win!)

### Where's the API docs?

Soon™

### Gib awoos

[Here you go](https://youtu.be/pKcR7qHlAIA?list=LL&t=75)
