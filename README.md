# League Tracker Discord Bot

League Tracker is a Discord app that "tracks" League of Legends players' ranked solo/duo queue matches and provides
updates on their performance. The bot is designed to be added to Discord servers where users can follow specific
summoners and receive notifications about their recent matches, including rank changes and game statistics.

## Features

- Track multiple League of Legends summoners in one place
- Automatically fetch and announce new ranked solo/duo queue matches
- Display detailed match information, including champion played, KDA, damage dealt, and more
- Show rank changes and LP gains/losses
- Maintain a history of tracked matches and summoner statistics
- Simple command interface for managing tracked summoners

## Tech Stack

- Go (Golang)
- PostgreSQL
- Discord API (via discordgo library)
- Riot Games API for League of Legends data

## Prerequisites

Before setting up the bot, make sure you have the following:

- Go 1.22
- PostgreSQL
- A Discord bot token - You can create one [here](https://discord.com/developers/applications/)
- A Riot Games API key - You can get a development api key [here](https://developer.riotgames.com/)

## Setup

1. Clone the repository:

   ```sh
   git clone https://github.com/tristan-derez/league-tracker.git
   cd league-tracker
   ```

2. Install dependencies:

   ```sh
   go mod tidy
   ```

3. Set up the environment variables: Create a `.env` file in the root directory with the content of the `.env.example`

4. You can skip 5 & 6 if you use docker. Just run:

   ```sh
   docker compose up
   ```

5. Set up the database: The bot will automatically create the necessary tables when it starts. Make sure your PostgreSQL
   server is running and accessible with the provided credentials.

6. Build and run the bot:
   ```sh
   go build -o league-tracker
   ./league-tracker
   ```

## Usage

Once the bot is running and added to your Discord server, you can use the following commands:

- `/add <summoner_name#tag>`: Add a summoner to track
- `/remove <summoner_name#tag>`: Remove a tracked summoner
- `/list`: List all tracked summoners in the server
- `/unchannel`: Remove the current channel as the update channel
- `/ping`: Check if the bot is online

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is open-source and available under the [MIT License](LICENSE).

## Support

If you encounter any issues or have questions, please open an issue on the GitHub repository.

---
