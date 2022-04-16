# SlimYTM
*A from-scratch implementation of the Logitech Media Server to stream music from Youtube Music*

Currently supports Squeezebox v1, 2 & 3

## Install
Clone the repo and then follow the instruction from ytmusicapi's [documentation](https://ytmusicapi.readthedocs.io/en/latest/setup.html) to copy your YTM auth headers.
Save the headers into `headers_auth.json`

## Usage
Start the Golang server (`go run .`) and the Python server (`python ytm.py`). Start your Squeezebox.
It should discover the SlimYTM server and prompt you to select it as the server. Complete the setup sequence.

To select music to play, visit the web interface at `http://localhost:9000` (or where your server is)

Note: SlimYTM listens on both TCP ports 9000 and 9001. Use of xPL requires a hub.
To communicate with the Squeezebox, SlimYTM uses TCP and UDP port 3483.
