import { createBot, Intents, startBot } from "https://deno.land/x/discordeno@13.0.0/mod.ts";
import "https://deno.land/x/dotenv@v3.2.2/load.ts";

const bot = createBot({
  token: Deno.env.get("DISCORD_TOKEN") as string,
  intents: Intents.Guilds | Intents.GuildMessages,
  events: {
    ready() {
      console.log("Successfully connected to gateway");
    },
  },
});

// // Another way to do events
// bot.events.messageCreate = function (b, message) {
//   // Process the message here with your command handler.
// };

await startBot(bot);
