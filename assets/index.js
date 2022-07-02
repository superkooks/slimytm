const Home = {
    template: `<div id="players" class="routerView">
    <p style="font-weight: bold">Choose a player</p>
    <player
        v-for="player in $store.state.players"
        :player="player"
        :key="player.id"
    ></player>
</div>`
}

const PlayerHome = {
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
        this.$store.commit("currentPlaylist", { title: "Loading...", trackCount: "0", duration: "0 seconds", songs: [] })

        this.$store.dispatch("getPlaylist", this.$route.params.id)
    },
    
    methods: {
        playSong(event) {
            this.$store.dispatch("playSong", {player: Number(this.$route.params.player), playlist: this.$store.state.currentPlaylist, song: event})
        }
    }
}

const routes = [
    { path: "/", component: Home },
    { path: "/player/:player/", component: PlayerHome },
    { path: "/player/:player/playlist/:id", component: Playlist }
]

const router = VueRouter.createRouter({
    history: VueRouter.createWebHistory(),
    routes
})

const app = Vue.createApp({})

app.config.devtools = true
app.use(router)
app.use(store)

app.component("player", {
    props: ["player"],
    template: `<div class="player" @click="this.$router.push('/player/'+player.id)">
    <p style="font-weight: bold;">{{ player.name }}</p>
    <p>{{ player.type }}</p>
</div>`
})

app.component("playlist-cover", {
    props: ["playlist"],
    template: `<div class="playlistCover" @click="this.$router.push('/player/'+this.$route.params.player+'/playlist/'+playlist.id)">
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
<div id="playerControls" v-if="Object.keys(playerState.song).length > 0 || playerState.loading">

    <div id="playerControlButtons">
        <span class="material-icons md-48" @click="$store.dispatch('previousSong', Number($route.params.player))">
            skip_previous
        </span>
        <span class="material-icons md-48" v-if="playerState.paused" @click="$store.dispatch('pauseSong', Number($route.params.player))">
            play_arrow
        </span>
        <span class="material-icons md-48" v-else @click="$store.dispatch('pauseSong', Number($route.params.player))">
            pause
        </span>
        <span class="material-icons md-48" @click="$store.dispatch('nextSong', Number($route.params.player))">
            skip_next
        </span>
    </div>
    
    <div id="currentSong"
        v-if="playerState.loading"
    >
        <p>Loading...</p>
    </div>

    <div id="currentSong" v-else>
        <img class="thumbnail" :src="playerState.song.thumbnails[0].url">
        <div id="currentSongInfo">
            <span class="title">{{ playerState.song.title }}</span>
            <p>
                <span class="artist">{{ playerState.song.artists[0].name }}</span>
                <span class="noHover" v-if="playerState.song.album != null">  -  </span>
                <span class="album">{{ playerState.song.album != null ? playerState.song.album.name : "" }}</span>
            </p>
        </div>
    </div>
    <div id="playerVolume">
        <input type="range" min="0" max="100" step="5" :value="playerState.volume" @input="setVolume">
    </div>
</div>`,

    mounted() {
        // Connect to the websocket and load current states of players
        console.log("Connecting to websocket")
        ws = new WebSocket("ws://"+window.location.hostname+":9001/ws")
        this.$store.commit("ws", ws)

        ws.onmessage = (event) => {
            e = JSON.parse(event.data)
            console.log(e)
            this.$store.commit("playerState", e)
        }

        ws.onopen = () => {
            console.log("Connected to websocket")
        }

        ws.onerror = () => {
            this.$store.commit("wsFailed")
        }
    },

    methods: {
        setVolume(event) {
            this.$store.dispatch("setVolume", {
                player: Number(this.$route.params.player),
                volume: Number(event.target.value),
            })
        }
    },

    computed: {
        playerState() {
            s = this.$store.getters.playerState(this.$route.params.player)

            if (this.$route.params.player == undefined || s == undefined) {
                return {
                    id: 0,
                    song: {},
                    paused: false,
                    loading: false,
                    volume: 0
                }
            }

            return s
        }
    }
})

app.mount("#app")