# Go Messenger App Demo

Source code of the blog posts "Building a Messenger App":

- [Part 1: Schema](https://nicolasparada.netlify.com/posts/go-messenger-schema/)
- [Part 2: OAuth](https://nicolasparada.netlify.com/posts/go-messenger-oauth/)
- [Part 3: Conversations](https://nicolasparada.netlify.com/posts/go-messenger-conversations/)
- [Part 4: Messages](https://nicolasparada.netlify.com/posts/go-messenger-messages/)
- [Part 5: Realtime Messages](https://nicolasparada.netlify.com/posts/go-messenger-realtime-messages/)
- [Part 6: Development Login](https://nicolasparada.netlify.com/posts/go-messenger-dev-login/)
- [Part 7: Access Page](https://nicolasparada.netlify.com/posts/go-messenger-access-page/)
- [Part 8: Home Page](https://nicolasparada.netlify.com/posts/go-messenger-home-page/)
- [Part 9: Conversation Page](https://nicolasparada.netlify.com/posts/go-messenger-conversation-page/)

[DEMO](https://go-messenger-demo.herokuapp.com/)

Get the code:
```bash
go get -u github.com/nicolasparada/go-messenger-demo
```

Copy the example `.env` file:
```
cp .env.example .env
```
Now, modify it with your own [GitHub client ID and secret](https://github.com/settings/applications/new). In the Github page, set a callback URL like so `http://localhost:3000/api/oauth/github/callback`.

Start database instance:
```bash
cockroach start-single-node --insecure --host 127.0.0.1
```

Create database schema, build and run:
```bash
cat schema.sql | cockroach sql --insecure
go build -o messenger
./messenger
```
