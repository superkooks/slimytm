const Home = {
    template: `<div id="playlists" class="routerView">
    <playlist-cover
        v-for="playlist in $store.state.playlists"
        :playlist="playlist"
        :key="playlist.id"
    ></playlist-cover>
</div>`,
    created() {
        this.$store.dispatch("updatePlaylists")
    }
}

const Playlist = {
    template: `<div id="paylist" class="routerView">
        <div id="playlistHeader">
            <img id="playlistThumbnail" :src="$store.state.currentPlaylist.thumbnail">
            <div id="playlistInfo">
                <h1 id="playlistTitle">{{ $store.state.currentPlaylist.title }}</h1>
                <p id="playlistCount">{{ $store.state.currentPlaylist.trackCount }} songs</p>
                <p id="playlistDuration">{{ $store.state.currentPlaylist.duration }}</p>
            </div>
        </div>
        <div id="songs">
            <hr>
            <song @playSong="playSong" v-for="song in $store.state.currentPlaylist.tracks" :song="song" :key="song.videoId"></song>
        </div>
</div>`,

    created() {
        this.$store.commit("setCurrentPlaylist", { title: "Loading...", trackCount: "0", duration: "0 seconds", songs: [] })

        this.$store.dispatch("getPlaylist", this.$route.params.id)
    },
    
    methods: {
        playSong(event) {
            this.$store.dispatch("playSong", {playlist: this.$store.state.currentPlaylist, song: event})
        }
    }
}

const store = Vuex.createStore({
    state() {
        return {
            playlists: [
                { id: "LM", title: "Your Likes", count: "Some", thumbnail: "https://www.gstatic.com/youtube/media/ytm/images/pbg/liked-songs-@576.png" },
            ],
            currentPlaylist: {},
            currentSong: {},
            paused: false,
            volume: 50,
        }
    },

    mutations: {
        setPlaylists(state, playlists) {
            state.playlists = playlists
        },
        setCurrentPlaylist(state, list) {
            state.currentPlaylist = list
        },
        setPlayerState(state, player) {
            state.currentSong = player.song
            state.paused = player.paused
            state.volume = player.volume
        }
    },
    actions: {
        updatePlaylists(context) {
            fetch("/api/library/playlists").then((resp) => {
                return resp.json()
            }).then((resp) => {
                context.commit("setPlaylists", resp)
            })
        },
        getPlaylist(context, id) {
            fetch("/api/playlist/"+id+"?limit=30").then((resp) => {
                return resp.json()
            }).then((resp) => {
                context.commit("setCurrentPlaylist", resp)
            })
        },
        playSong(context, e) {
            data = {queueType: "playlist", queueId: e.playlist.id, startSong: e.song}
            fetch("http://"+window.location.hostname+":9001/play", {
                method: "POST",
                body: JSON.stringify(data),
                headers: {
                    'Content-Type': 'application/json'
                }
            }).catch(() => {
                console.log("failed to play song")
            })
        },
    }
})

const routes = [
    { path: "/", component: Home },
    { path: "/playlist/:id", component: Playlist }
]

const router = VueRouter.createRouter({
    history: VueRouter.createWebHistory(),
    routes
})

const app = Vue.createApp({})

app.config.devtools = true
app.use(router)
app.use(store)

app.component("playlist-cover", {
    props: ["playlist"],
    template: `<div class="playlistCover" @click="this.$router.push('/playlist/'+playlist.id)">
    <img :src="playlist.thumbnail">
    <span class="playlistTitle">{{ playlist.title }}</span>
    <span class="playlistCount">{{ playlist.count }} songs</span>
</div>`
})

app.component("song", {
    props: ["song"],
    emits: ["playSong"],
    template: `<div class="song">
    <img class="thumbnail" @click="$emit('playSong', song)" :src="song.thumbnails[0].url">
    <div class="title"><span @click="$emit('playSong', song)">{{ song.title }}</span></div>
    <div class="artist"><span>{{ song.artists[0].name }}</span></div>
    <div class="album"><span>{{ song.album != null ? song.album.name : "" }}</span></div>
    <div class="duration"><span class="noHover">{{ song.duration }}</span></div>
</div>
<hr>`
})

app.component("player-controls", {
    template: `<hr>
    <div id="playerControls" v-if="Object.keys($store.state.currentSong).length > 0">
    <div id="playerControlButtons">
        <svg class="playButton" viewBox="0 0 24 24" preserveAspectRatio="xMidYMid meet">
            <g class="style-scope tp-yt-iron-icon">
                <path d="M8 5v14l11-7z" class="style-scope tp-yt-iron-icon"></path>
            </g>
        </svg>
    </div>
    <div id="currentSong">
        <img class="thumbnail" :src="$store.state.currentSong.thumbnails[0].url">
        <div id="currentSongInfo">
            <span class="title">{{ $store.state.currentSong.title }}</span>
            <p>
                <span class="artist">{{ $store.state.currentSong.artists[0].name }}</span>
                <span class="noHover" v-if="$store.state.currentSong.album != null">  -  </span>
                <span class="album">{{ $store.state.currentSong.album != null ? $store.state.currentSong.album.name : "" }}</span>
            </p>
        </div>
    </div>
    <div id="playerVolume">
    </div>
</div>`,

    created() {
        // Connect to the websocket to listen for events
        console.log("Connecting to websocket")
        ws = new WebSocket("ws://"+window.location.hostname+":9001/ws")

        ws.onmessage = (event) => {
            e = JSON.parse(event.data)
            console.log(e)
            this.$store.commit("setPlayerState", e)
        }

        ws.onopen = () => {
            console.log("Connected to websocket")
        }
    }
})

app.mount("#app")