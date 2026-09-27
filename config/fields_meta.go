package config

// Descriptions and editor hints for every option, keyed by dotted path.
// TestEveryFieldDocumented fails when an option is added to Config without
// an entry here, so the config editor never shows an unexplained field.

const (
	warnRestartPorts = "Players connect to this port: change the firewall and any published address with it."
	warnDatabase     = "Erupe will not start if it cannot reach the database with these settings."
	warnDebug        = "Development option: leave off on a public server."
)

var fieldMeta = map[string]meta{
	// General
	"Host":                      {desc: "Address players connect to, sent to the client in the server list. Empty = detect this machine's outbound IPv4."},
	"BinPath":                   {desc: "Folder holding quest, scenario and Hunting Road data. \"bin\" keeps using an existing bin/ folder, otherwise game-data/."},
	"ContentPath":               {desc: "Folder of <table>/*.json content files (shops, exchanges, event quests…) synchronised into the database on start and on content reload. Empty = <BinPath>/content."},
	"Language":                  {desc: "Default language of server-side messages (chat commands, notices). Players can pick their own with the language command.", options: []string{"en", "jp", "fr", "es", "zh"}},
	"DisableShutdownCountdown":  {desc: "Shut down immediately instead of counting down in game first, so scripts can restart the server unattended."},
	"ShutdownCountdownSeconds":  {desc: "Seconds of in-game countdown before a shutdown. Ignored when the countdown is disabled.", min: bound(0)},
	"ShutdownDrainSeconds":      {desc: "Extra seconds to wait for players to disconnect on their own before closing their sessions.", min: bound(0)},
	"HideLoginNotice":           {desc: "Do not show the login notices below when a player logs in."},
	"LoginNotices":              {desc: "Notices shown at login, one entry per page, in MHFML markup (<BODY>, <BR>, <CENTER>, <SIZE_3>, <C_4>…). Notices created from the admin API are added after these."},
	"PatchServerManifest":       {desc: "Bare host (no http://) the original launcher asks for /mhf_file.php. Needed for PS3 clients."},
	"PatchServerFile":           {desc: "Bare host (no http://) the original launcher downloads /mhfdat/ files from. Usually the same as the manifest host."},
	"DeleteOnSaveCorruption":    {desc: "Flag a character for deletion when the client sends a corrupted save. Can be used against cheaters.", warning: "Deletes characters: a client bug can trigger it too."},
	"DisableSaveIntegrityCheck": {desc: "Skip the SHA-256 check of saves at load. Only needed when importing saves from another server.", warning: "Without the check, a corrupted save is loaded instead of falling back to a backup."},
	"ClientMode":                {desc: "Game version the server speaks. ZZ is the primary target; versions up to G10.1 are for debugging only.", options: versionStrings},
	"QuestCacheExpiry":          {desc: "Seconds a decoded event quest stays in memory before it is read from disk again.", min: bound(0)},
	"CommandPrefix":             {desc: "Character that starts a chat command, as in !help."},
	"AutoCreateAccount":         {desc: "Create an account automatically the first time an unknown username logs in.", warning: "Anyone who can reach the server can create accounts."},
	"LoopDelay":                 {desc: "Milliseconds each session waits between two passes of its packet loop. Lower is more responsive and uses more CPU.", min: bound(1)},
	"DefaultCourses":            {desc: "Course IDs every account always has (1 Trial, 2 Hunter Life, 3 Extra, 6 Premium, 26 NetCafe…; see the course enumeration on the wiki)."},
	"EarthStatus":               {desc: "Global event state: 1–2 Conquest War, 11 Pallone Fest, 12 Pallone rewards, 21 Tower. 0 = none."},
	"EarthID":                   {desc: "Identifier of the running global event."},
	"EarthMonsters":             {desc: "Monster IDs targeted during Conquest War."},

	// SaveDumps
	"SaveDumps.Enabled":    {desc: "Write a copy of every save the client sends to the output folder."},
	"SaveDumps.RawEnabled": {desc: "Also write the decompressed save next to each copy."},
	"SaveDumps.OutputDir":  {desc: "Folder save copies are written to."},

	// Screenshots
	"Screenshots.Enabled":       {desc: "Accept screenshots uploaded from the game's BBS feature."},
	"Screenshots.Host":          {desc: "Host the client uploads screenshots to."},
	"Screenshots.Port":          {desc: "Port the client uploads screenshots to (normally the API port)."},
	"Screenshots.OutputDir":     {desc: "Folder screenshots are stored in."},
	"Screenshots.UploadQuality": {desc: "JPEG quality of stored screenshots, from 1 to 100.", min: bound(1), max: bound(100)},

	// Capture
	"Capture.Enabled":         {desc: "Record network traffic to .mhfr files for protocol research."},
	"Capture.OutputDir":       {desc: "Folder capture files are written to."},
	"Capture.ExcludeOpcodes":  {desc: "Packet opcodes left out of captures (pings, positions…)."},
	"Capture.CaptureSign":     {desc: "Record sign server sessions."},
	"Capture.CaptureEntrance": {desc: "Record entrance server sessions."},
	"Capture.CaptureChannel":  {desc: "Record channel server sessions."},

	// DebugOptions
	"DebugOptions.CleanDB":             {desc: "Delete all users, characters and guilds when the server starts.", warning: "Wipes every account and character on each start."},
	"DebugOptions.MaxLauncherHR":       {desc: "Report HR7 to the launcher so any character can enter worlds with an HR requirement.", warning: warnDebug},
	"DebugOptions.LogInboundMessages":  {desc: "Log every packet received from clients.", warning: "Produces very large logs."},
	"DebugOptions.LogOutboundMessages": {desc: "Log every packet sent to clients.", warning: "Produces very large logs."},
	"DebugOptions.LogMessageData":      {desc: "Add a hex dump of each logged packet.", warning: "Produces very large logs."},
	"DebugOptions.MaxHexdumpLength":    {desc: "Bytes of each packet printed in hex dumps.", min: bound(0)},
	"DebugOptions.DivaOverride":        {desc: "Force the Diva Defense state: -1 = follow the event schedule, 0 = off, 1–3 = phase."},
	"DebugOptions.FestaOverride":       {desc: "Force the Hunter Festa state: -1 = follow the event schedule, 0 = off, 1–3 = phase."},
	"DebugOptions.TournamentOverride":  {desc: "Reserved for forcing the VS Tournament state. Not read by the server yet."},
	"DebugOptions.DisableTokenCheck":   {desc: "Accept any login token without checking it against the database.", warning: "Lets anyone log in as anyone. Never enable on a public server."},
	"DebugOptions.QuestTools":          {desc: "Log quest loading details."},
	"DebugOptions.AutoQuestBackport":   {desc: "Convert quest files to the layout of older clients when ClientMode is older than ZZ."},
	"DebugOptions.ProxyPort":           {desc: "Send clients to a channel proxy on this port (mhf-dev-proxy). 0 = off.", warning: warnDebug},
	"DebugOptions.CapLink.Values":      {desc: "Values the client checks for the CapLink service."},
	"DebugOptions.CapLink.Key":         {desc: "Secret key of the CapLink service.", secret: true},
	"DebugOptions.CapLink.Host":        {desc: "Address of the CapLink service."},
	"DebugOptions.CapLink.Port":        {desc: "Port of the CapLink service."},

	// GameplayOptions
	"GameplayOptions.MinFeatureWeapons":              {desc: "Minimum number of Active Feature weapons drawn each day.", min: bound(0)},
	"GameplayOptions.MaxFeatureWeapons":              {desc: "Maximum number of Active Feature weapons drawn each day.", min: bound(0)},
	"GameplayOptions.MaximumNP":                      {desc: "Most N Points a player can hold.", min: bound(0)},
	"GameplayOptions.MaximumRP":                      {desc: "Most Road Points a player can hold."},
	"GameplayOptions.RPAccrualNormalSeconds":         {desc: "Seconds of play per Road Point earned, without the cafe course.", min: bound(1), max: bound(2147483647)},
	"GameplayOptions.RPAccrualCafeSeconds":           {desc: "Seconds of play per Road Point earned, with the cafe course.", min: bound(1), max: bound(2147483647)},
	"GameplayOptions.MaximumFP":                      {desc: "Most Festa Points a player can hold."},
	"GameplayOptions.TreasureHuntExpiry":             {desc: "Seconds before a Clan Treasure Hunt expires."},
	"GameplayOptions.TreasureHuntPartnyaCooldown":    {desc: "Seconds before a Partnya can join another Clan Treasure Hunt."},
	"GameplayOptions.DisableLoginBoost":              {desc: "Turn off the Login Boost bonuses."},
	"GameplayOptions.DisableBoostTime":               {desc: "Turn off the daily NetCafe Boost Time."},
	"GameplayOptions.BoostTimeDuration":              {desc: "Seconds the daily NetCafe Boost Time lasts.", min: bound(0)},
	"GameplayOptions.ClanMealDuration":               {desc: "Seconds a cooked Clan Meal can be activated for.", min: bound(0)},
	"GameplayOptions.ClanMemberLimits":               {desc: "Clan size by clan rank, as [rank, members] pairs, e.g. [[0, 30], [3, 40]]. At most 100 members."},
	"GameplayOptions.BonusQuestAllowance":            {desc: "Bonus Point quests allowed per day."},
	"GameplayOptions.DailyQuestAllowance":            {desc: "Daily quests allowed per day."},
	"GameplayOptions.LowLatencyRaviente":             {desc: "Update Raviente's shared HP instantly. Uses more network traffic."},
	"GameplayOptions.RegularRavienteMaxPlayers":      {desc: "Players allowed in Regular Raviente (HR2)."},
	"GameplayOptions.ViolentRavienteMaxPlayers":      {desc: "Players allowed in Violent Raviente (HR5)."},
	"GameplayOptions.BerserkRavienteMaxPlayers":      {desc: "Players allowed in Berserk Raviente (G1)."},
	"GameplayOptions.ExtremeRavienteMaxPlayers":      {desc: "Players allowed in Extreme Raviente (G1, G50 weapon)."},
	"GameplayOptions.SmallBerserkRavienteMaxPlayers": {desc: "Players allowed in Small Berserk Raviente (G1)."},
	"GameplayOptions.GUrgentRate":                    {desc: "Chance of G Urgent quests appearing, from 0 to 1.", min: bound(0), max: bound(1)},
	"GameplayOptions.GCPMultiplier":                  {desc: "Multiplier on GCP earned from quests.", min: bound(0)},
	"GameplayOptions.HRPMultiplier":                  {desc: "Multiplier on Hunter Rank Points earned from quests.", min: bound(0)},
	"GameplayOptions.HRPMultiplierNC":                {desc: "Multiplier on Hunter Rank Points earned from quests with the NetCafe bonus.", min: bound(0)},
	"GameplayOptions.SRPMultiplier":                  {desc: "Multiplier on Skill Rank Points earned from quests.", min: bound(0)},
	"GameplayOptions.SRPMultiplierNC":                {desc: "Multiplier on Skill Rank Points earned from quests with the NetCafe bonus.", min: bound(0)},
	"GameplayOptions.GRPMultiplier":                  {desc: "Multiplier on G Rank Points earned from quests.", min: bound(0)},
	"GameplayOptions.GRPMultiplierNC":                {desc: "Multiplier on G Rank Points earned from quests with the NetCafe bonus.", min: bound(0)},
	"GameplayOptions.GSRPMultiplier":                 {desc: "Multiplier on G Skill Rank Points earned from quests.", min: bound(0)},
	"GameplayOptions.GSRPMultiplierNC":               {desc: "Multiplier on G Skill Rank Points earned from quests with the NetCafe bonus.", min: bound(0)},
	"GameplayOptions.ZennyMultiplier":                {desc: "Multiplier on zenny earned from quests.", min: bound(0)},
	"GameplayOptions.ZennyMultiplierNC":              {desc: "Multiplier on zenny earned from quests with the NetCafe bonus.", min: bound(0)},
	"GameplayOptions.GZennyMultiplier":               {desc: "Multiplier on zenny earned from G Rank quests.", min: bound(0)},
	"GameplayOptions.GZennyMultiplierNC":             {desc: "Multiplier on zenny earned from G Rank quests with the NetCafe bonus.", min: bound(0)},
	"GameplayOptions.MaterialMultiplier":             {desc: "Multiplier on monster materials rewarded by quests.", min: bound(0)},
	"GameplayOptions.MaterialMultiplierNC":           {desc: "Multiplier on monster materials rewarded by quests with the NetCafe bonus.", min: bound(0)},
	"GameplayOptions.GMaterialMultiplier":            {desc: "Multiplier on monster materials rewarded by G Rank quests.", min: bound(0)},
	"GameplayOptions.GMaterialMultiplierNC":          {desc: "Multiplier on monster materials rewarded by G Rank quests with the NetCafe bonus.", min: bound(0)},
	"GameplayOptions.ExtraCarves":                    {desc: "Extra carves on every carcass."},
	"GameplayOptions.ExtraCarvesNC":                  {desc: "Extra carves on every carcass with the NetCafe bonus."},
	"GameplayOptions.GExtraCarves":                   {desc: "Extra carves on every G Rank carcass."},
	"GameplayOptions.GExtraCarvesNC":                 {desc: "Extra carves on every G Rank carcass with the NetCafe bonus."},
	"GameplayOptions.DisableHunterNavi":              {desc: "Turn off the Hunter Navi."},
	"GameplayOptions.MezFesSoloTickets":              {desc: "MezFes solo tickets given each week."},
	"GameplayOptions.MezFesGroupTickets":             {desc: "MezFes group tickets given each week."},
	"GameplayOptions.MezFesDuration":                 {desc: "Seconds MezFes lasts each week, counted back from Monday 00:00.", min: bound(0)},
	"GameplayOptions.MezFesSwitchMinigame":           {desc: "Replace Volpakkun Together with Tokotoko Partnya as the group minigame."},
	"GameplayOptions.EnableKaijiEvent":               {desc: "Run the Kaiji event in the Rasta Bar (G10 only)."},
	"GameplayOptions.EnableHiganjimaEvent":           {desc: "Run the Higanjima event in the Rasta Bar."},
	"GameplayOptions.EnableNierEvent":                {desc: "Run the NieR event in the Rasta Bar."},
	"GameplayOptions.DisableRoad":                    {desc: "Close the Hunting Road."},
	"GameplayOptions.SeasonOverride":                 {desc: "Make quests follow the current Mezeporta season and time of day."},

	// Discord
	"Discord.Enabled":                       {desc: "Connect the Discord bot (account linking, password reset, chat relay)."},
	"Discord.BotToken":                      {desc: "Token of the Discord bot.", secret: true},
	"Discord.RelayChannel.Enabled":          {desc: "Relay in-game chat to a Discord channel and back."},
	"Discord.RelayChannel.MaxMessageLength": {desc: "Longest Discord message relayed into the game, in characters.", min: bound(1)},
	"Discord.RelayChannel.RelayChannelID":   {desc: "ID of the Discord channel chat is relayed to."},

	// BinSync
	"BinSync.Enabled":     {desc: "Offer to download quest, scenario and Hunting Road data from a remote manifest."},
	"BinSync.ManifestURL": {desc: "HTTP(S) address of the remote manifest.json."},

	// Lists of structures
	"Commands": {desc: "Chat commands: name, enabled, description and the word typed after the prefix."},
	"Courses":  {desc: "Courses players may toggle with the course command."},

	// Database
	"Database.Host":     {desc: "PostgreSQL host.", warning: warnDatabase},
	"Database.Port":     {desc: "PostgreSQL port.", warning: warnDatabase},
	"Database.User":     {desc: "PostgreSQL user.", warning: warnDatabase},
	"Database.Password": {desc: "PostgreSQL password.", secret: true, warning: warnDatabase},
	"Database.Database": {desc: "PostgreSQL database name.", warning: warnDatabase},

	// Servers
	"Sign.Enabled":     {desc: "Run the sign server (logins)."},
	"Sign.Port":        {desc: "Port of the sign server.", warning: warnRestartPorts},
	"Channel.Enabled":  {desc: "Run the channel servers (gameplay)."},
	"Entrance.Enabled": {desc: "Run the entrance server (world list)."},
	"Entrance.Port":    {desc: "Port of the entrance server.", warning: warnRestartPorts},
	"Entrance.Entries": {desc: "Worlds shown in the world list, each with its type (1 open, 2 cities, 3 newbie, 4 tavern, 5 return, 6 MezFes), name, and channel ports and capacities."},

	// API
	"API.Enabled":             {desc: "Run the web API (launcher, sign-up, dashboard, this editor)."},
	"API.Port":                {desc: "Port of the web API.", warning: "This editor is served on this port: after a restart, open it at the new port."},
	"API.PatchServer":         {desc: "Origin of the game-file tree advertised to mhf-outpost. Empty with PatchTree on = this server."},
	"API.PatchTree.Enabled":   {desc: "Serve the game files (/mhf_file.php and /mhfdat/) from this server."},
	"API.PatchTree.Root":      {desc: "Folder holding key.txt and mhfdat/{exe,dat} for the patch tree."},
	"API.Banners":             {desc: "Launcher banners: image URL and link."},
	"API.Messages":            {desc: "Launcher messages: text, date, kind (0 normal, 1 new) and link."},
	"API.Links":               {desc: "Launcher links: name, icon URL and link."},
	"API.LandingPage.Enabled": {desc: "Show the landing page at the server's web root."},
	"API.LandingPage.Title":   {desc: "Title of the landing page."},
	"API.LandingPage.Content": {desc: "Body of the landing page, in HTML."},
}
