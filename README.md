# AI Subs

This was a test project to experiment with working with local AI models. It is built to look through my media library to look for srt files (subtitles) and pretend to be a grumpy old man to provide a summary. It then puts that summary into an nfo file so that it can be read by my Jellyfin server. Additionally, it uses the The Movie Database API to go look up the movie title.

As this was built for my specific use case, it relies on the media directory containing the srt file to have something like "[tmdbid-1771]" in the name. If it doesn't find one, it will assume the directory name is the media name.

## Running

Before you can run this application, you need to have a couple environment variables defined

| Env Variable | Description | Example |
|-|-|-|
|`OLLAMA_URL`| The URL to access your Ollama API | `http://localhost:11434/api/generate` |
|`TMDB_TOKEN`| The read token (not API key) for your [The Movie Database](https://www.themoviedb.org/) account | `eyJh-and-a-bunch-more-random-letters-M` |
|`MEDIA_DIR`| The location where your media lives | `~/media/movies` |

With those setup, you can download the latest executable from the [Releases](https://github.com/sturdy5/ai-subs/releases/). Running the `ai-subs` executable will go ahead and create your summaries in your `MEDIA_DIR`.

### Setting up Ollama

If you are also trying this out to see what its like to use local models, you can download Ollama from [here](http://ollama.com/download/). When you download it, you can run `ollama run llama3.2:1b`. That will open up a chat, but it will also initialize the API. You can exit out (`/exit`) of Ollama an the API will stay running.
