package simulation

// Gear catalogs organized by category. Each persona draws from different
// categories based on their type (see personas.go for assignment logic).

// ToolsGear contains hand and power tools for home projects.
var ToolsGear = []GearTemplate{
	{Name: "Cordless Drill", Description: "18V cordless drill/driver with two batteries", Category: "Tools", Brand: "DeWalt", Model: "DCD771C2", MaterialCategory: "mixed_plastic_metal", WeightGrams: 1800, ValueUSD: 99},
	{Name: "Circular Saw", Description: "7-1/4 inch circular saw for framing and general cutting", Category: "Tools", Brand: "Makita", Model: "5007MGA", MaterialCategory: "mixed_plastic_metal", WeightGrams: 4800, ValueUSD: 149},
	{Name: "Orbital Sander", Description: "Random orbit sander for finishing work", Category: "Tools", Brand: "Bosch", Model: "ROS20VSC", MaterialCategory: "mixed_plastic_metal", WeightGrams: 1300, ValueUSD: 69},
	{Name: "Jigsaw", Description: "Variable speed jigsaw for curved cuts", Category: "Tools", Brand: "Milwaukee", Model: "2737-20", MaterialCategory: "mixed_plastic_metal", WeightGrams: 2300, ValueUSD: 179},
	{Name: "Impact Driver", Description: "Compact impact driver for driving screws and bolts", Category: "Tools", Brand: "DeWalt", Model: "DCF887B", MaterialCategory: "mixed_plastic_metal", WeightGrams: 900, ValueUSD: 119},
	{Name: "Reciprocating Saw", Description: "Heavy-duty reciprocating saw for demolition", Category: "Tools", Brand: "Milwaukee", Model: "2821-20", MaterialCategory: "mixed_plastic_metal", WeightGrams: 3200, ValueUSD: 199},
	{Name: "Table Saw", Description: "10-inch portable table saw with folding stand", Category: "Tools", Brand: "DeWalt", Model: "DWE7491RS", MaterialCategory: "metal", WeightGrams: 26000, ValueUSD: 599},
	{Name: "Miter Saw", Description: "10-inch compound sliding miter saw", Category: "Tools", Brand: "Makita", Model: "LS1019L", MaterialCategory: "metal", WeightGrams: 14000, ValueUSD: 449},
	{Name: "Router", Description: "Variable speed plunge router for edge work and joints", Category: "Tools", Brand: "Bosch", Model: "MRP23EVS", MaterialCategory: "mixed_plastic_metal", WeightGrams: 5200, ValueUSD: 219},
	{Name: "Nail Gun", Description: "18-gauge cordless brad nailer", Category: "Tools", Brand: "Ryobi", Model: "P320", MaterialCategory: "mixed_plastic_metal", WeightGrams: 2700, ValueUSD: 149},
	{Name: "Angle Grinder", Description: "4.5-inch angle grinder for cutting and grinding metal", Category: "Tools", Brand: "Makita", Model: "GA4530", MaterialCategory: "mixed_plastic_metal", WeightGrams: 1800, ValueUSD: 49},
	{Name: "Stud Finder", Description: "Electronic stud finder with deep scanning", Category: "Tools", Brand: "Franklin", Model: "ProSensor T13", MaterialCategory: "plastic", WeightGrams: 200, ValueUSD: 59},
	{Name: "Laser Level", Description: "Self-leveling cross-line laser level", Category: "Tools", Brand: "Bosch", Model: "GCL100-80C", MaterialCategory: "plastic", WeightGrams: 700, ValueUSD: 199},
}

// KitchenGear contains kitchen appliances and specialty items.
var KitchenGear = []GearTemplate{
	{Name: "Stand Mixer", Description: "5-quart tilt-head stand mixer for baking", Category: "Kitchen", Brand: "KitchenAid", Model: "Artisan KSM150PS", MaterialCategory: "metal", WeightGrams: 11800, ValueUSD: 349},
	{Name: "Food Processor", Description: "14-cup food processor with multiple blades", Category: "Kitchen", Brand: "Cuisinart", Model: "DFP-14BCWN", MaterialCategory: "plastic", WeightGrams: 5400, ValueUSD: 199},
	{Name: "Pressure Cooker", Description: "8-quart electric pressure cooker and slow cooker", Category: "Kitchen", Brand: "Instant Pot", Model: "Duo 80", MaterialCategory: "metal", WeightGrams: 5300, ValueUSD: 89},
	{Name: "Immersion Blender", Description: "Hand blender with whisk and chopper attachments", Category: "Kitchen", Brand: "Breville", Model: "BSB510XL", MaterialCategory: "mixed_plastic_metal", WeightGrams: 900, ValueUSD: 99},
	{Name: "Bread Machine", Description: "Programmable bread maker with multiple loaf sizes", Category: "Kitchen", Brand: "Zojirushi", Model: "BB-PDC20", MaterialCategory: "mixed_plastic_metal", WeightGrams: 8200, ValueUSD: 349},
	{Name: "Pasta Maker", Description: "Hand-crank pasta roller and cutter set", Category: "Kitchen", Brand: "Marcato", Model: "Atlas 150", MaterialCategory: "metal", WeightGrams: 2200, ValueUSD: 79},
	{Name: "Waffle Iron", Description: "Belgian waffle maker with deep pockets", Category: "Kitchen", Brand: "Cuisinart", Model: "WAF-F20", MaterialCategory: "metal", WeightGrams: 3600, ValueUSD: 69},
	{Name: "Dehydrator", Description: "5-tray food dehydrator for jerky and dried fruit", Category: "Kitchen", Brand: "Nesco", Model: "FD-75A", MaterialCategory: "plastic", WeightGrams: 3200, ValueUSD: 69},
	{Name: "Ice Cream Maker", Description: "2-quart automatic ice cream maker", Category: "Kitchen", Brand: "Cuisinart", Model: "ICE-30BC", MaterialCategory: "mixed_plastic_metal", WeightGrams: 5000, ValueUSD: 69},
	{Name: "Juicer", Description: "Centrifugal juicer for fruits and vegetables", Category: "Kitchen", Brand: "Breville", Model: "JE98XL", MaterialCategory: "mixed_plastic_metal", WeightGrams: 4500, ValueUSD: 149},
}

// OutdoorGear contains camping, hiking, and outdoor recreation items.
var OutdoorGear = []GearTemplate{
	{Name: "4-Person Tent", Description: "3-season dome tent with rainfly, sleeps 4", Category: "Outdoor", Brand: "REI Co-op", Model: "Half Dome 4 Plus", MaterialCategory: "textile", WeightGrams: 3200, ValueUSD: 329},
	{Name: "Kayak", Description: "10-foot recreational sit-in kayak", Category: "Outdoor", Brand: "Pelican", Model: "Mustang 100X", MaterialCategory: "plastic", WeightGrams: 18000, ValueUSD: 299},
	{Name: "Bike Rack", Description: "Hitch-mounted bike rack for 4 bikes", Category: "Outdoor", Brand: "Thule", Model: "T2 Pro XTR", MaterialCategory: "metal", WeightGrams: 23000, ValueUSD: 649},
	{Name: "Camping Stove", Description: "2-burner propane camping stove", Category: "Outdoor", Brand: "Coleman", Model: "Classic 2-Burner", MaterialCategory: "metal", WeightGrams: 5400, ValueUSD: 69},
	{Name: "Cooler", Description: "65-quart rotomolded cooler, keeps ice 7+ days", Category: "Outdoor", Brand: "YETI", Model: "Tundra 65", MaterialCategory: "plastic", WeightGrams: 14400, ValueUSD: 375},
	{Name: "Camping Hammock", Description: "Double camping hammock with tree straps", Category: "Outdoor", Brand: "ENO", Model: "DoubleNest", MaterialCategory: "textile", WeightGrams: 600, ValueUSD: 69},
	{Name: "Headlamp", Description: "Rechargeable headlamp with 1000 lumens", Category: "Outdoor", Brand: "Petzl", Model: "Actik Core", MaterialCategory: "plastic", WeightGrams: 75, ValueUSD: 69},
	{Name: "Trekking Poles", Description: "Adjustable carbon fiber trekking poles (pair)", Category: "Outdoor", Brand: "Black Diamond", Model: "Distance Carbon Z", MaterialCategory: "carbon_fiber", WeightGrams: 280, ValueUSD: 169},
	{Name: "Sleeping Bag", Description: "20-degree mummy sleeping bag, 650-fill down", Category: "Outdoor", Brand: "REI Co-op", Model: "Magma 20", MaterialCategory: "textile", WeightGrams: 900, ValueUSD: 299},
	{Name: "Portable Grill", Description: "Tabletop propane gas grill", Category: "Outdoor", Brand: "Weber", Model: "Q1200", MaterialCategory: "metal", WeightGrams: 12000, ValueUSD: 209},
	{Name: "Paddleboard", Description: "Inflatable stand-up paddleboard with paddle and pump", Category: "Outdoor", Brand: "iROCKER", Model: "All Around 11'", MaterialCategory: "plastic", WeightGrams: 10000, ValueUSD: 499},
}

// GardenGear contains lawn and garden equipment.
var GardenGear = []GearTemplate{
	{Name: "Lawn Mower", Description: "21-inch self-propelled gas lawn mower", Category: "Garden", Brand: "Honda", Model: "HRN216VKA", MaterialCategory: "metal", WeightGrams: 38000, ValueUSD: 449},
	{Name: "Chainsaw", Description: "18-inch gas chainsaw for tree work", Category: "Garden", Brand: "Husqvarna", Model: "450 Rancher", MaterialCategory: "mixed_plastic_metal", WeightGrams: 5200, ValueUSD: 399},
	{Name: "Leaf Blower", Description: "Cordless leaf blower with 56V battery", Category: "Garden", Brand: "EGO", Model: "LB5804", MaterialCategory: "plastic", WeightGrams: 3200, ValueUSD: 229},
	{Name: "Pressure Washer", Description: "2100 PSI electric pressure washer", Category: "Garden", Brand: "Ryobi", Model: "RY142300", MaterialCategory: "mixed_plastic_metal", WeightGrams: 15000, ValueUSD: 249},
	{Name: "String Trimmer", Description: "Cordless string trimmer/edger combo", Category: "Garden", Brand: "EGO", Model: "ST1521S", MaterialCategory: "mixed_plastic_metal", WeightGrams: 2700, ValueUSD: 199},
	{Name: "Hedge Trimmer", Description: "24-inch cordless hedge trimmer", Category: "Garden", Brand: "Black+Decker", Model: "LHT2436", MaterialCategory: "mixed_plastic_metal", WeightGrams: 3100, ValueUSD: 99},
	{Name: "Rototiller", Description: "16-inch electric garden tiller/cultivator", Category: "Garden", Brand: "Earthwise", Model: "TC70016", MaterialCategory: "metal", WeightGrams: 12000, ValueUSD: 149},
	{Name: "Wheelbarrow", Description: "6 cubic foot steel wheelbarrow", Category: "Garden", Brand: "True Temper", Model: "R6FF25", MaterialCategory: "metal", WeightGrams: 18000, ValueUSD: 89},
}

// PartyGear contains event and entertainment equipment.
var PartyGear = []GearTemplate{
	{Name: "Projector", Description: "1080p portable projector for movie nights", Category: "Party", Brand: "Epson", Model: "Home Cinema 880", MaterialCategory: "mixed_plastic_metal", WeightGrams: 2700, ValueUSD: 349},
	{Name: "Bluetooth Speaker", Description: "Waterproof portable Bluetooth speaker", Category: "Party", Brand: "JBL", Model: "Xtreme 3", MaterialCategory: "plastic", WeightGrams: 1900, ValueUSD: 349},
	{Name: "Karaoke Machine", Description: "Portable karaoke system with wireless mics", Category: "Party", Brand: "Singing Machine", Model: "SDL9040", MaterialCategory: "mixed_plastic_metal", WeightGrams: 5400, ValueUSD: 149},
	{Name: "Projector Screen", Description: "100-inch portable projector screen with stand", Category: "Party", Brand: "Elite Screens", Model: "Yard Master 2", MaterialCategory: "textile", WeightGrams: 8000, ValueUSD: 199},
	{Name: "String Lights", Description: "100-foot commercial grade patio string lights", Category: "Party", Brand: "Brightech", Model: "Ambience Pro", MaterialCategory: "plastic", WeightGrams: 2200, ValueUSD: 49},
	{Name: "Canopy Tent", Description: "10x10 pop-up canopy with sidewalls", Category: "Party", Brand: "E-Z UP", Model: "Envoy", MaterialCategory: "metal", WeightGrams: 20000, ValueUSD: 249},
	{Name: "Folding Tables", Description: "Set of two 6-foot folding tables", Category: "Party", Brand: "Lifetime", Model: "22900", MaterialCategory: "plastic", WeightGrams: 20000, ValueUSD: 130},
	{Name: "Cooler Jockey Box", Description: "Double-tap jockey box for serving cold beverages", Category: "Party", Brand: "Coldbreak", Model: "Jockey Box 2T", MaterialCategory: "mixed_plastic_metal", WeightGrams: 9000, ValueUSD: 299},
}

// ElectronicsGear contains consumer electronics and tech items.
var ElectronicsGear = []GearTemplate{
	{Name: "DSLR Camera", Description: "Entry-level DSLR with 18-55mm lens", Category: "Electronics", Brand: "Canon", Model: "EOS Rebel T7", MaterialCategory: "mixed_plastic_metal", WeightGrams: 900, ValueUSD: 479},
	{Name: "Drone", Description: "Compact camera drone with 4K video", Category: "Electronics", Brand: "DJI", Model: "Mini 3", MaterialCategory: "plastic", WeightGrams: 250, ValueUSD: 469},
	{Name: "VR Headset", Description: "Standalone VR headset for gaming", Category: "Electronics", Brand: "Meta", Model: "Quest 3", MaterialCategory: "plastic", WeightGrams: 515, ValueUSD: 499},
	{Name: "Portable Monitor", Description: "15.6-inch portable USB-C monitor", Category: "Electronics", Brand: "ASUS", Model: "ZenScreen MB16AC", MaterialCategory: "mixed_plastic_metal", WeightGrams: 780, ValueUSD: 249},
	{Name: "Label Maker", Description: "Portable thermal label maker", Category: "Electronics", Brand: "Brother", Model: "P-Touch PTD220", MaterialCategory: "plastic", WeightGrams: 480, ValueUSD: 39},
	{Name: "Telescope", Description: "Computerized telescope for stargazing", Category: "Electronics", Brand: "Celestron", Model: "NexStar 5SE", MaterialCategory: "mixed_plastic_metal", WeightGrams: 13000, ValueUSD: 999},
}

// AllGear returns all gear templates across all categories.
func AllGear() []GearTemplate {
	var all []GearTemplate
	all = append(all, ToolsGear...)
	all = append(all, KitchenGear...)
	all = append(all, OutdoorGear...)
	all = append(all, GardenGear...)
	all = append(all, PartyGear...)
	all = append(all, ElectronicsGear...)
	return all
}
