![logo](https://github.com/donuts-are-good/imghost/assets/96031819/46c17259-549a-4395-ab10-a4c5814f974e)
![donuts-are-good's followers](https://img.shields.io/github/followers/donuts-are-good?&color=555&style=for-the-badge&label=followers) ![donuts-are-good's stars](https://img.shields.io/github/stars/donuts-are-good?affiliations=OWNER%2CCOLLABORATOR&color=555&style=for-the-badge) ![donuts-are-good's visitors](https://komarev.com/ghpvc/?username=donuts-are-good&color=555555&style=for-the-badge&label=visitors)


# loba
loba is a load balancer written in go. it's experimental so maybe dont use it at work yet.

# what
loba uses a round-robin algorithm to distribute incoming http requests across multiple servers. it also logs each request and response, storing the data in a sqlite database. you can then generate reports on the total number of requests per domain and the most recent request time.

# domains config
all you need to do is create a domains.json file in the config directory. here's an example:

```json
[
  {
    "domain": "example1.com",
    "servers": [
      "http://server1.example.com",
      "http://server2.example.com"
    ]
  },
  {
    "domain": "example2.com",
    "servers": [
      "http://server3.example.com",
      "http://server4.example.com"
    ]
  }
]
```

each object in the array represents a domain and the servers that will handle requests for that domain.

# health check
loba also includes a health check endpoint at /health. it's a simple way to verify that the load balancer is up and running.

# reporting
want to see how your servers are doing? just hit the /report endpoint. you'll get a json response with the total number of requests and the most recent request time for each domain.

# neosay config
here's an example of a neosay config:

```json
{
  "HomeserverURL": "https://matrix.org",
  "UserID": "@myusername:matrix.org",
  "AccessToken": "sy_uegfiubwefiouh98h38u4hgr34uy",
  "Rooms": {
    "error": "!NvGwefy34y3lIy:matrix.org",
    "general": "!yzLf3445hheeQR:matrix.org",
    "request": "!Cydgojiihergsgk:matrix.org",
    "launch": "!WwehbewuybgweaPC:matrix.org"
  }
}
```

MIT License 2023 donuts-are-good, for more info see license.md
