// Automatically downloads and configures Steam grid images for all games in a
// given Steam installation.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Prints an error and quits.
func errorAndExit(err error) {
	fmt.Println(err.Error())
	bufio.NewReader(os.Stdin).ReadBytes('\n')
	os.Exit(0)
}

func main() {
	http.DefaultTransport.(*http.Transport).ResponseHeaderTimeout = time.Second * 10
	startApplication()
}

func bToMb(b uint64) uint64 {
	return b / 1024 / 1024
}

func printMemStats(endline ...bool) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// For info on each, see: https://golang.org/pkg/runtime/#MemStats
	fmt.Printf("Alloc = %v MiB", bToMb(m.Alloc))
	fmt.Printf("\tTotalAlloc = %v MiB", bToMb(m.TotalAlloc))
	//fmt.Printf("\tSys = %v MiB", bToMb(m.Sys))
	//fmt.Printf("\tNumGC = %v", m.NumGC)

	if len(endline) == 0 || endline[0] {
		fmt.Printf("\n")
	}
}

func startApplication() {
	steamGridDBApiKey := flag.String("steamgriddb", "", "Your personal SteamGridDB api key, get one here: https://www.steamgriddb.com/profile/preferences")
	IGDBSecret := flag.String("igdbsecret", "", "Your personal IGDB api key, get one here: https://api.igdb.com/signup")
	IGDBClient := flag.String("igdbclient", "", "Your personal IGDB api key, get one here: https://api.igdb.com/signup")
	steamDir := flag.String("steamdir", "", "Path to your steam installation")
	// "alternate" "blurred" "white_logo" "material" "no_logo"
	steamGridDBStyles := flag.String("styles", "alternate", "Comma separated list of styles to download from SteamGridDB.\nExample: \"white_logo,material\"")
	steamGridDBLogoStyles := flag.String("logostyles", "official", "Comma separated list of styles to download from SteamGridDB.\nExample: \"white,black\"")
	// "static" "animated"
	steamGridDBTypes := flag.String("types", "static", "Comma separated list of types to download from SteamGridDB.\nExample: \"static,animated\"")
	steamGridDBNsfw := flag.String("nsfw", "false", "Set to false to filter out nsfw, true to only include nsfw, any to include both.")
	steamGridDBHumor := flag.String("humor", "false", "Set to false to filter out humor, true to only include humor, any to include both.")
	steamGridDBBannerDimensions := flag.String("bannerdimensions", "460x215,920x430", "Filter results by image dimensions. Multiple dimensions can be provided as comma seperated strings.")
	steamGridDBCoverDimensions := flag.String("coverdimensions", "600x900,342x482,660x930", "Filter results by image dimensions. Multiple dimensions can be provided as comma seperated strings.")
	steamGridDBHeroDimensions := flag.String("herodimensions", "1920x620,3840x1240,1600x650", "Filter results by image dimensions. Multiple dimensions can be provided as comma seperated strings.")
	skipSteam := flag.Bool("skipsteam", false, "Skip downloads from Steam servers")
	skipGoogle := flag.Bool("skipgoogle", false, "Skip search and downloads from google")
	skipBanner := flag.Bool("skipbanner", false, "Skip search and processing banner artwork")
	skipCover := flag.Bool("skipcover", false, "Skip search and processing cover artwork")
	skipHero := flag.Bool("skiphero", false, "Skip search and processing hero artwork")
	skipLogo := flag.Bool("skiplogo", false, "Skip search and processing logo artwork")
	nonSteamOnly := flag.Bool("nonsteamonly", false, "Only search artwork for Non-Steam-Games")
	appIDs := flag.String("appids", "", "Comma separated list of appIds that should be processed")
	onlyMissingArtwork := flag.Bool("onlymissingartwork", false, "Only download artworks missing on the official servers")
	convertWebpToApng := flag.Bool("webpasapng", false, "Convert WEBP animations to APNG.\nMakes them load faster in Steam but takes longer to apply.")
	convertWebpToApngCoversBanners := flag.Bool("coverwebpasapng", false, "Convert only WEBP animations to APNG (only covers and banners)\nAvoid Hero and Logo which may be too memory and time consuming to apply.")
	flag.Parse()
	if flag.NArg() == 1 {
		steamDir = &flag.Args()[0]
	} else if flag.NArg() >= 2 {
		flag.Usage()
		os.Exit(1)
	}

	// Process command line flags
	steamGridDBBannerFilter := "?styles=" + *steamGridDBStyles + "&types=" + *steamGridDBTypes + "&nsfw=" + *steamGridDBNsfw + "&humor=" + *steamGridDBHumor + "&dimensions=" + *steamGridDBBannerDimensions
	steamGridDBCoverFilter := "?styles=" + *steamGridDBStyles + "&types=" + *steamGridDBTypes + "&nsfw=" + *steamGridDBNsfw + "&humor=" + *steamGridDBHumor + "&dimensions=" + *steamGridDBCoverDimensions
	steamGridDBHeroFilter := "?styles=" + *steamGridDBStyles + "&types=" + *steamGridDBTypes + "&nsfw=" + *steamGridDBNsfw + "&humor=" + *steamGridDBHumor + "&dimensions=" + *steamGridDBHeroDimensions
	steamGridDBLogoFilter := "?styles=" + *steamGridDBLogoStyles + "&types=" + *steamGridDBTypes + "&nsfw=" + *steamGridDBNsfw + "&humor=" + *steamGridDBHumor

	artStyles := map[string][]string{
		// artStyle: ["idExtension", "nameExtension", steamUrlExtension, steamGridDbFilter]
		"Banner": []string{"", ".banner", "header.jpg", steamGridDBBannerFilter},
		"Cover":  []string{"p", ".cover", "library_600x900_2x.jpg", steamGridDBCoverFilter},
		"Hero":   []string{"_hero", ".hero", "library_hero.jpg", steamGridDBHeroFilter},
		"Logo":   []string{"_logo", ".logo", "logo.png", steamGridDBLogoFilter},
	}

	if *skipBanner {
		delete(artStyles, "Banner")
	}
	if *skipCover {
		delete(artStyles, "Cover")
	}
	if *skipHero {
		delete(artStyles, "Hero")
	}
	if *skipLogo {
		delete(artStyles, "Logo")
	}
	if len(artStyles) == 0 {
		errorAndExit(errors.New("No artStyles, nothing to do…"))
	}

	if *skipSteam && *onlyMissingArtwork {
		errorAndExit(errors.New("Can't check if official artwork is missing with steam turned off"))
	}

	fmt.Println("Loading overlays...")
	overlays, err := LoadOverlays(filepath.Join(filepath.Dir(os.Args[0]), "overlays by category"), artStyles)
	if err != nil {
		errorAndExit(err)
	}
	if len(overlays) == 0 {
		fmt.Println("No category overlays found. You can put overlay images in the folder 'overlays by category', where the filename is the game category.\n\nYou can find many user-created overlays at https://www.reddit.com/r/steamgrid/wiki/overlays .\n\nContinuing without overlays...")
	} else {
		fmt.Printf("Loaded %v overlays. \n\nYou can find many user-created overlays at https://www.reddit.com/r/steamgrid/wiki/overlays .\n\n", len(overlays))
	}

	fmt.Println("Looking for Steam directory...\nIf SteamGrid doesn´t find the directory automatically, launch it with an argument linking to the Steam directory.")
	installationDir, err := GetSteamInstallation(*steamDir)
	if err != nil {
		errorAndExit(err)
	}

	fmt.Println("Loading users...")
	users, err := GetUsers(installationDir)
	if err != nil {
		errorAndExit(err)
	}
	if len(users) == 0 {
		errorAndExit(errors.New("No users found at Steam/userdata. Have you used Steam before in this computer?"))
	}

	nOverlaysApplied := 0
	nDownloaded := 0
	notFounds := map[string][]*Game{
		"Banner": []*Game{},
		"Cover":  []*Game{},
		"Hero":   []*Game{},
		"Logo":   []*Game{},
	}
	steamGridDB := map[string][]*Game{
		"Banner": []*Game{},
		"Cover":  []*Game{},
		"Hero":   []*Game{},
		"Logo":   []*Game{},
	}
	IGDB := map[string][]*Game{
		"Banner": []*Game{},
		"Cover":  []*Game{},
		"Hero":   []*Game{},
		"Logo":   []*Game{},
	}
	searchedGames := map[string][]*Game{
		"Banner": []*Game{},
		"Cover":  []*Game{},
		"Hero":   []*Game{},
		"Logo":   []*Game{},
	}
	failedGames := map[string][]*Game{
		"Banner": []*Game{},
		"Cover":  []*Game{},
		"Hero":   []*Game{},
		"Logo":   []*Game{},
	}
	var errorMessages []string

	for _, user := range users {
		fmt.Println("Loading games for " + user.Name)
		gridDir := filepath.Join(user.Dir, "config", "grid")

		err = os.MkdirAll(filepath.Join(gridDir, "originals"), 0777)
		if err != nil {
			errorAndExit(err)
		}

		games := GetGames(user, *nonSteamOnly, *appIDs)

		fmt.Println("Loading existing images and backups...")

		i := 0
		for _, game := range games {
			i++

			var name string
			if game.Name == "" {
				game.Name = getGameName(game.ID)
			}

			if game.Name != "" {
				name = game.Name
			} else {
				name = "unknown game with id " + game.ID
			}

			fmt.Printf("Processing %v (%v/%v)\n", name, i, len(games))

			for artStyle, artStyleExtensions := range artStyles {
				// Clear for multiple runs:
				game.ImageSource = ""
				game.ImageExt = ""
				game.CleanImageBytes = nil
				game.OverlayImageBytes = nil

				overridePath := filepath.Join(filepath.Dir(os.Args[0]), "games")
				loadExisting(overridePath, gridDir, game, artStyleExtensions)
				// This cleans up unused backups and images for the same game but with different extensions.
				err = removeExisting(gridDir, game.ID, artStyleExtensions)
				if err != nil {
					fmt.Println(err.Error())
				}

				///////////////////////
				// Download if missing.
				///////////////////////
				if game.ImageSource == "" {
					from, err := DownloadImage(gridDir, game, artStyle, artStyleExtensions, *skipSteam, *steamGridDBApiKey, *IGDBSecret, *IGDBClient, *skipGoogle, *onlyMissingArtwork)
					if err != nil && err.Error() == "SteamGridDB authorization token is missing or invalid" {
						// Wrong api key
						*steamGridDBApiKey = ""
						fmt.Println(err.Error())
					} else if err != nil {
						fmt.Println(err.Error())
					}

					if game.ImageSource == "" {
						notFounds[artStyle] = append(notFounds[artStyle], game)
						fmt.Printf("%v not found\n", artStyle)
						// Game has no image, skip it.
						continue
					} else if err == nil {
						nDownloaded++
					}

					switch from {
					case "IGDB":
						IGDB[artStyle] = append(IGDB[artStyle], game)
					case "SteamGridDB":
						steamGridDB[artStyle] = append(steamGridDB[artStyle], game)
					case "search":
						searchedGames[artStyle] = append(searchedGames[artStyle], game)
					}
				}
				fmt.Printf("%v found from %v\n", artStyle, game.ImageSource)

				///////////////////////
				// Apply overlay.
				//
				// Expecting name.artExt.imgExt:
				// Banner: favorites.png
				// Cover: favorites.p.png
				// Hero: favorites.hero.png
				// Logo: favorites.logo.png
				///////////////////////
				err := ApplyOverlay(game, overlays, artStyleExtensions, *convertWebpToApng, *convertWebpToApngCoversBanners)
				if err != nil {
					print(err.Error(), "\n")
					failedGames[artStyle] = append(failedGames[artStyle], game)
					errorMessages = append(errorMessages, err.Error())
				}
				if game.OverlayImageBytes != nil {
					nOverlaysApplied++
				} else {
					game.OverlayImageBytes = game.CleanImageBytes
				}

				///////////////////////
				// Save result.
				///////////////////////
				err = backupGame(gridDir, game, artStyleExtensions)
				if err != nil {
					errorAndExit(err)
				}

				if strings.Contains(game.ImageExt, "webp") {
					game.ImageExt = ".png"
				}

				imagePath := filepath.Join(gridDir, game.ID+artStyleExtensions[0]+game.ImageExt)
				err = ioutil.WriteFile(imagePath, game.OverlayImageBytes, 0666)

				// Copy with legacy naming for Big Picture mode
				if artStyle == "Banner" {
					// use appID
					id, err := strconv.ParseUint(game.ID, 10, 64)
					if game.LegacyID != 0 {
						// old target+exe format for custom shortcuts
						id = game.LegacyID
					}
					if err == nil {
						imagePath := filepath.Join(gridDir, strconv.FormatUint(id<<32|0x02000000, 10)+artStyleExtensions[0]+game.ImageExt)
						err = ioutil.WriteFile(imagePath, game.OverlayImageBytes, 0666)
					}
				}
				if err != nil {
					fmt.Printf("Failed to write image for %v (%v) because: %v\n", game.Name, artStyle, err.Error())
				}
			}
		}
	}

	fmt.Printf("\n\n%v images downloaded and %v overlays applied.\n\n", nDownloaded, nOverlaysApplied)
	if len(searchedGames["Banner"])+len(searchedGames["Cover"])+len(searchedGames["Hero"])+len(searchedGames["Logo"]) >= 1 {
		fmt.Printf("%v images were found with a Google search and may not be accurate:\n", len(searchedGames["Banner"])+len(searchedGames["Cover"])+len(searchedGames["Hero"])+len(searchedGames["Logo"]))
		for artStyle, games := range searchedGames {
			for _, game := range games {
				fmt.Printf("* %v (steam id %v, %v)\n", game.Name, game.ID, artStyle)
			}
		}

		fmt.Printf("\n\n")
	}

	if len(IGDB["Banner"])+len(IGDB["Cover"]) >= 1 {
		fmt.Printf("%v images were found on IGDB and may not be in full quality or accurate:\n", len(IGDB["Banner"])+len(IGDB["Cover"]))
		for artStyle, games := range IGDB {
			for _, game := range games {
				fmt.Printf("* %v (steam id %v, %v)\n", game.Name, game.ID, artStyle)
			}
		}

		fmt.Printf("\n\n")
	}

	if len(steamGridDB["Banner"])+len(steamGridDB["Cover"])+len(steamGridDB["Hero"])+len(steamGridDB["Logo"]) >= 1 {
		fmt.Printf("%v images were found on SteamGridDB and may not be in full quality or accurate:\n", len(steamGridDB["Banner"])+len(steamGridDB["Cover"])+len(steamGridDB["Hero"])+len(steamGridDB["Logo"]))
		for artStyle, games := range steamGridDB {
			for _, game := range games {
				fmt.Printf("* %v (steam id %v, %v)\n", game.Name, game.ID, artStyle)
			}
		}

		fmt.Printf("\n\n")
	}

	if len(notFounds["Banner"])+len(notFounds["Cover"])+len(notFounds["Hero"])+len(notFounds["Logo"]) >= 1 {
		fmt.Printf("%v images could not be found anywhere:\n", len(notFounds["Banner"])+len(notFounds["Cover"])+len(notFounds["Hero"])+len(notFounds["Logo"]))
		for artStyle, games := range notFounds {
			for _, game := range games {
				fmt.Printf("- %v (id %v, %v)\n", game.Name, game.ID, artStyle)
			}
		}

		fmt.Printf("\n\n")
	}

	if len(failedGames["Banner"])+len(failedGames["Cover"])+len(failedGames["Hero"])+len(failedGames["Logo"]) >= 1 {
		fmt.Printf("%v images were found but had errors and could not be overlaid:\n", len(failedGames["Banner"])+len(failedGames["Cover"])+len(failedGames["Hero"])+len(failedGames["Logo"]))
		for artStyle, games := range failedGames {
			var i = 0
			for _, game := range games {
				fmt.Printf("- %v (id %v, %v) (%v)\n", game.Name, game.ID, artStyle, errorMessages[i])
				i++
			}
		}

		fmt.Printf("\n\n")
	}

	fmt.Println("Open Steam in grid view to see the results!\n\nPress enter to close.")

	bufio.NewReader(os.Stdin).ReadBytes('\n')
}
