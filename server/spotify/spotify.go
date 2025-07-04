package spotify

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"os"
	"time"

	"github.com/tidwall/gjson"
)

/* for playlists and albums */
type ResourceEndpoint struct {
	Limit, Offset, TotalCount, Requests int64
}

type Track struct {
	Title, Artist, Album string
	Artists              []string
	Duration             int
}

const (
	
	tokenEndpoint       = "https://open.spotify.com/get_access_token?reason=transport&productType=web-player"
	trackInitialPath    = "https://api-partner.spotify.com/pathfinder/v1/query?operationName=getTrack&variables="
	playlistInitialPath = "https://api-partner.spotify.com/pathfinder/v1/query?operationName=fetchPlaylist&variables="
	albumInitialPath    = "https://api-partner.spotify.com/pathfinder/v1/query?operationName=getAlbum&variables="
	trackEndPath        = `{"persistedQuery":{"version":1,"sha256Hash":"e101aead6d78faa11d75bec5e36385a07b2f1c4a0420932d374d89ee17c70dd6"}}`
	playlistEndPath     = `{"persistedQuery":{"version":1,"sha256Hash":"b39f62e9b566aa849b1780927de1450f47e02c54abf1e66e513f96e849591e41"}}`
	albumEndPath        = `{"persistedQuery":{"version":1,"sha256Hash":"46ae954ef2d2fe7732b4b2b4022157b2e18b7ea84f70591ceb164e4de1b5d5d3"}}`
)

var 	clientID     = os.Getenv("clientID")
var     clientSecret= os.Getenv("clientSecret")

func getAccessToken() (string, error) {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}

	// Base64 encode clientID:clientSecret
	auth := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
	req.Header.Add("Authorization", "Basic "+auth)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get access token: %s", body)
	}

	var result struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return "", err
	}

	return result.AccessToken, nil
}

/* requests to playlist/track endpoints */
func request(endpoint string) (int, string, error) {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return 0, "", fmt.Errorf("error on making the request")
	}

	bearer, err := getAccessToken()
	if err != nil {
		return 0, "", fmt.Errorf("failed to get access token: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+bearer)

	resp, err := (&http.Client{}).Do(req)
	fmt.Printf("%d",resp);
	if err != nil {
		return 0, "", fmt.Errorf("error on getting response: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", fmt.Errorf("error on reading response: %w", err)
	}

	return resp.StatusCode, string(body), nil
}

func getID(url string) string {
	parts := strings.Split(url, "/")
	id := strings.Split(parts[4], "?")[0]
	return id
}

func isValidPattern(url, pattern string) bool {
	match, _ := regexp.MatchString(pattern, url)
	return match
}

func TrackInfo(url string) (*Track, error) {
	trackPattern := `^https:\/\/open\.spotify\.com\/track\/[a-zA-Z0-9]{22}(\?.*)?$`
	if !isValidPattern(url, trackPattern) {
		return nil, errors.New("invalid track url")
	}

	id := getID(url)
	endpoint := "https://api.spotify.com/v1/tracks/" + id

	statusCode, jsonResponse, err := request(endpoint)
	if err != nil {
		return nil, fmt.Errorf("error on getting track info: %w", err)
	}

	if statusCode != 200 {
		return nil, fmt.Errorf("received non-200 status code: %d", statusCode)
	}

	// Now parse official Web API response structure
	var track Track
	track.Title = gjson.Get(jsonResponse, "name").String()
	track.Album = gjson.Get(jsonResponse, "album.name").String()
	track.Duration = int(gjson.Get(jsonResponse, "duration_ms").Int() / 1000)

	artists := gjson.Get(jsonResponse, "artists").Array()
	for _, artist := range artists {
		name := artist.Get("name").String()
		if name != "" {
			track.Artists = append(track.Artists, name)
		}
	}
	if len(track.Artists) > 0 {
		track.Artist = track.Artists[0]
	}

	return track.buildTrack(), nil
}
func PlaylistInfo(url string) ([]Track, error) {
    playlistPattern := `^https:\/\/open\.spotify\.com\/playlist\/[a-zA-Z0-9]{22}(\?.*)?$`
    if !isValidPattern(url, playlistPattern) {
        return nil, errors.New("invalid playlist url")
    }

    id := getID(url)
    // Use market=IN as requested
    endpoint := fmt.Sprintf("https://api.spotify.com/v1/playlists/%s/tracks?market=IN", id)
    fmt.Printf("%s\n", endpoint)

    statusCode, jsonResponse, err := request(endpoint)
    if err != nil {
        return nil, fmt.Errorf("error on getting playlist info: %w", err)
    }
    if statusCode != 200 {
        return nil, fmt.Errorf("received non-200 status code: %d", statusCode)
    }

    // Parse tracks from the response
    var tracks []Track
    items := gjson.Get(jsonResponse, "items").Array()
    for _, item := range items {
        track := item.Get("track")
        title := track.Get("name").String()
        album := track.Get("album.name").String()
        duration := int(track.Get("duration_ms").Int() / 1000)
        var artists []string
        for _, artist := range track.Get("artists").Array() {
            artists = append(artists, artist.Get("name").String())
        }
        mainArtist := ""
        if len(artists) > 0 {
            mainArtist = artists[0]
        }
        tracks = append(tracks, Track{
            Title:    title,
            Album:    album,
            Duration: duration,
            Artists:  artists,
            Artist:   mainArtist,
        })
    }

    return tracks, nil
}
func AlbumInfo(url string) ([]Track, error) {
    albumPattern := `^https:\/\/open\.spotify\.com\/album\/[a-zA-Z0-9]{22}(\?.*)?$`
    if !isValidPattern(url, albumPattern) {
        return nil, errors.New("invalid album url")
    }

    id := getID(url)
    // Use market=IN as requested
    endpoint := fmt.Sprintf("https://api.spotify.com/v1/albums/%s/tracks?market=IN", id)
    fmt.Printf("%s\n", endpoint)

    statusCode, jsonResponse, err := request(endpoint)
    if err != nil {
        return nil, fmt.Errorf("error on getting album info: %w", err)
    }
    if statusCode != 200 {
        return nil, fmt.Errorf("received non-200 status code: %d", statusCode)
    }

    // Parse tracks from the response
    var tracks []Track
    items := gjson.Get(jsonResponse, "items").Array()
    for _, item := range items {
        title := item.Get("name").String()
        album := "" // Album name is not in the track object here, you can fetch it separately if needed
        duration := int(item.Get("duration_ms").Int() / 1000)
        var artists []string
        for _, artist := range item.Get("artists").Array() {
            artists = append(artists, artist.Get("name").String())
        }
        mainArtist := ""
        if len(artists) > 0 {
            mainArtist = artists[0]
        }
        tracks = append(tracks, Track{
            Title:    title,
            Album:    album,
            Duration: duration,
            Artists:  artists,
            Artist:   mainArtist,
        })
    }

    return tracks, nil
}
/* returns playlist/album slice of tracks */
func resourceInfo(url, resourceType, totalCount, itemList string) ([]Track, error) {
	id := getID(url)
	eConf := ResourceEndpoint{Limit: 400, Offset: 0}
	jsonResponse, err := jsonList(resourceType, id, eConf.Offset, eConf.Limit)
	if err != nil {
		return nil, err
	}

	eConf.TotalCount = gjson.Get(jsonResponse, totalCount).Int()

	if eConf.TotalCount < 1 {
		return nil, errors.New("hum, there are no tracks")
	}

	name := map[bool]string{true: gjson.Get(jsonResponse, "data.playlistV2.name").String(), false: gjson.Get(jsonResponse, "data.albumUnion.name").String()}[resourceType == "playlist"]
	fmt.Printf("Collecting tracks from '%s'...\n", name)
	time.Sleep(1 * time.Second)

	eConf.Requests = int64(math.Ceil(float64(eConf.TotalCount) / float64(eConf.Limit))) /* total of requests */
	var tracks []Track
	tracks = append(tracks, proccessItems(jsonResponse, resourceType)...)

	for i := 1; i < int(eConf.Requests); i++ {
		eConf.pagination()

		jsonResponse, err := jsonList(resourceType, id, eConf.Offset, eConf.Limit)
		if err != nil {
			return nil, err
		}
		tracks = append(tracks, proccessItems(jsonResponse, resourceType)...)
	}

	fmt.Println("Tracks collected:", len(tracks))
	return tracks, nil
}

/* gets JSON respond from playlist/album endpoints */
func jsonList(resourceType, id string, offset, limit int64) (string, error) {
	var endpointQuery string
	var endpoint string
	if resourceType == "playlist" {
		endpointQuery = EncodeParam(fmt.Sprintf(`{"uri":"spotify:playlist:%s","offset":%d,"limit":%d}`, id, offset, limit))
		endpoint = playlistInitialPath + endpointQuery + "&extensions=" + EncodeParam(playlistEndPath)
	} else {
		endpointQuery = EncodeParam(fmt.Sprintf(`{"uri":"spotify:album:%s","locale":"","offset":%d,"limit":%d}`, id, offset, limit))
		endpoint = albumInitialPath + endpointQuery + "&extensions=" + EncodeParam(albumEndPath)
	}

	statusCode, jsonResponse, err := request(endpoint)
	if err != nil {
		return "", fmt.Errorf("error getting tracks: %w", err)
	}

	if statusCode != 200 {
		return "", fmt.Errorf("received non-200 status code: %d", statusCode)
	}

	return jsonResponse, nil
}

func (t *Track) buildTrack() *Track {
	track := &Track{
		Title:    t.Title,
		Artist:   t.Artist,
		Artists:  t.Artists,
		Duration: t.Duration,
		Album:    t.Album,
	}

	return track
}

func (eConf *ResourceEndpoint) pagination() {
	eConf.Offset = eConf.Offset + eConf.Limit
}

/* constructs each Spotify track from JSON body (album/playlist) and returns a slice of tracks */
func proccessItems(jsonResponse, resourceType string) []Track {
	itemList := map[bool]string{true: "data.playlistV2.content.items", false: "data.albumUnion.tracks.items"}[resourceType == "playlist"]
	songTitle := map[bool]string{true: "itemV2.data.name", false: "track.name"}[resourceType == "playlist"]
	artistName := map[bool]string{true: "itemV2.data.artists.items.0.profile.name", false: "track.artists.items.0.profile.name"}[resourceType == "playlist"]
	albumName := map[bool]string{true: "itemV2.data.albumOfTrack.name", false: "data.albumUnion.name"}[resourceType == "playlist"]
	duration := map[bool]string{true: "itemV2.data.trackDuration.totalMilliseconds", false: "track.duration.totalMilliseconds"}[resourceType == "playlist"]

	var tracks []Track
	items := gjson.Get(jsonResponse, itemList).Array()

	for _, item := range items {
		durationInSeconds := int(item.Get(duration).Int()) / 1000

		track := &Track{
			Title:    item.Get(songTitle).String(),
			Artist:   item.Get(artistName).String(),
			Duration: durationInSeconds,
			Album:    map[bool]string{true: item.Get(albumName).String(), false: gjson.Get(jsonResponse, albumName).String()}[resourceType == "playlist"],
		}
		tracks = append(tracks, *track.buildTrack())
	}

	return tracks
}
