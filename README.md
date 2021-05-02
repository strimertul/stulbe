# stulbe - strimertul backend

Back-end portion of strimertul for features that require one:

- Streamer "is live" checks

Planned modules include:

- Webhook subscription for alerts
- Loyalty tracking, APIs for redeems etc

Platform support is limited to Twitch only for the time being (sorry!)

## Building

You need to build the frontend first!

```sh
cd frontend
npm i
npm run build
```

Once that's done, just build the app like any other Go project

```sh
go build
```

## License

The entire project is licensed under [AGPL-3.0-only](LICENSE) (see `LICENSE`).
