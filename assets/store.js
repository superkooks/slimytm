const store = Vuex.createStore({
    state() {
        return {
            playlists: [
                { id: "LM", title: "Your Likes", count: "Some", thumbnail: "https://www.gstatic.com/youtube/media/ytm/images/pbg/liked-songs-@576.png" },
            ],
            currentPlaylist: {},

            players: [],
            ws: null,
            wsFailed: false,
        }
    },

    mutations: {
        players(state, players) {
            state.players = players
        },
        currentPlayer(state, player) {
            state.currentPlayer = player
        },

        playlists(state, playlists) {
            state.playlists = playlists
        },
        currentPlaylist(state, list) {
            state.currentPlaylist = list
        },

        playerState(state, player) {
            state.players = state.players.filter((e) => {return e.id != player.id})
            state.players.push(player)
        },
        ws(state, ws) {
            state.ws = ws
        },
        wsFailed(state) {
            state.wsFailed = true
        }
    },
    actions: {
        // HTTP GETs
        updatePlaylists(context) {
            fetch("/api/library/playlists").then((resp) => {
                return resp.json()
            }).then((resp) => {
                context.commit("playlists", resp)
            })
        },
        getPlaylist(context, id) {
            fetch("/api/playlist/"+id+"?limit=30").then((resp) => {
                return resp.json()
            }).then((resp) => {
                context.commit("currentPlaylist", resp)
            })
        },

        // WS Sends
        playSong(context, e) {
            data = {queueType: "playlist", queueId: e.playlist.id, startSong: e.song, shuffle: false}
            context.state.ws.send(JSON.stringify({type: "PLAY", player: e.player, data: data}))
        },
        shuffle(context, e) {
            data = {queueType: "playlist", queueId: e.playlist.id, startSong: e.song, shuffle: true}
            context.state.ws.send(JSON.stringify({type: "PLAY", player: e.player, data: data}))
        },
        setVolume(context, e) {
            context.state.ws.send(JSON.stringify({type: "VOLUME", player: e.player, data: e.volume}))
        },
        nextSong(context, player) {
            context.state.ws.send(JSON.stringify({type: "NEXT", player: player}))
        },
        previousSong(context, player) {
            context.state.ws.send(JSON.stringify({type: "PREVIOUS", player: player}))
        },
        pauseSong(context, player) {
            context.state.ws.send(JSON.stringify({type: "PAUSE", player: player}))
        }
    },
    getters: {
        playerState: (state) => (id) => {
            return state.players.filter(v => {return v.id == id})[0]
        }
    }
})
