# NickCast

**NickCast** is a lightweight Go application that enables user authentication for streaming servers using IRC NickServ credentials. Designed with simplicity and nostalgia in mind, NickCast brings back the spirit of the old Icecast and Shoutcast days --- but with modern IRC integration.

Currently, **NickCast** supports the [Ergo IRC server](https://ergo.chat)'s NickServ authentication API.

---

## ✨ Features

- 🔐 **IRC-based streaming authentication** (via NickServ)
- 🔄 Automatically validates streamers using their IRC credentials
- 🔒 Bans and restrictions from IRC also apply to stream access
- 🛠️ Planned API support for stream moderation (kick, cooldowns, bans)

---

## 📦 Requirements

- Go 1.20+
- A running [Ergo IRC server](https://ergo.chat)
- A NickServ API endpoint with a valid bearer token
- A streaming client such as `ezstream`, `ices`, or `ffmpeg`

---

## ⚙️ Configuration

NickCast expects a simple config file named `nickcast.conf` in the same directory as the compiled binary:

```conf
# Server listen address (host:port)
listen = :8000

# NickServ API endpoint
auth_url = http://localhost:8089/v1/check_auth

# Bearer token for the NickServ API
api_token = your-ergo-api-bearer-token-here

```

* * * * *

🚀 Running
----------

1.  **Build the binary**:

    ```
    go build -o nickcast ./cmd/nickcast/main.go

    ```

2.  **Place your `nickcast.conf`** in the same directory as `nickcast`.

3.  **Start the server**:

    ```
    ./nickcast

    ```

4.  **Configure your streaming client**
    Since most icecast/shoutcast software only takes a password, use NickServ auth by entering your passsword as `<nick>:<password>`.

* * * * *

🎯 Why NickCast?
----------------

Back in the day, running a community Icecast or Shoutcast server meant constantly rotating stream passwords and managing permissions manually. With NickCast, authentication is seamless and tied directly to your IRC identity --- if you're banned in IRC, you're banned from the stream too.

* * * * *

🛠️ Future Plans
----------------

-   ✅ Stream user authentication via NickServ

-   🔜 API for kicking or timing out streamers

-   🔜 Automatic rejections for recently kicked users (cooldown)

-   ✅ IRC ban sync: banned IRC users are denied stream access

-   🔜 Support for more IRC servers (not just Ergo)

* * * * *

 me know if you'd like a logo badge, CI instructions, or systemd unit to add to it as well.

```
