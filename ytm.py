from flask.helpers import send_from_directory
from ytmusicapi import YTMusic
import flask
import json

ytmusic = YTMusic("headers_auth.json")
app = flask.Flask(__name__)


@app.route("/api/library/playlists")
def libraryPlaylists():
    playlists = ytmusic.get_library_playlists()
    playlists[0]["count"] = "Some"
    for p in playlists:
        p["id"] = p["playlistId"]
        del p["playlistId"]
        p["thumbnail"] = p["thumbnails"][1]["url"]
        del p["thumbnails"]

    return json.dumps(playlists, indent=2)


@app.route("/api/playlist/<id>")
def playlist(id):
    limit = flask.request.args.get("limit", 9999999999, type=int)
    start = flask.request.args.get("start", 0, type=int)
    playlist = ytmusic.get_playlist(id, limit=limit)
    playlist["thumbnail"] = playlist["thumbnails"][1]["url"]
    del playlist["thumbnails"]
    playlist["tracks"] = playlist["tracks"][start:]

    return json.dumps(playlist, indent=2)


@app.route("/assets/<path>")
def assets(path):
    return send_from_directory("assets", path)


@app.errorhandler(404)
def catchAllIndex(path):
    # Hooking 404 means that we can load on vue paths
    # that don't exist on the server
    return send_from_directory("assets", "index.html")


if __name__ == "__main__":
    app.run(host='0.0.0.0', port=9000)
