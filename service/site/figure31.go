package site

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/lib/pq"
	"github.com/mikeydub/go-gallery/graphql/dataloader"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/multichain"
	"github.com/mikeydub/go-gallery/service/persist"
	sentryutil "github.com/mikeydub/go-gallery/service/sentry"
	"github.com/spf13/viper"
)

const transferHash = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"

// Mapping of mint ID to position in collection
var tokenPositions = map[int]int{
	103: 0,
	193: 1,
	236: 2,
	397: 3,
	56:  4,
	178: 5,
	221: 6,
	420: 7,
	421: 8,
	26:  9,
	300: 10,
	332: 11,
	216: 12,
	336: 13,
	457: 14,
	119: 15,
	158: 16,
	183: 17,
	102: 18,
	222: 19,
	117: 20,
	259: 21,
	70:  22,
	34:  23,
	245: 24,
	169: 25,
	89:  26,
	470: 27,
	482: 28,
	469: 29,
	283: 30,
	129: 31,
	51:  32,
	452: 33,
	490: 34,
	163: 35,
	133: 36,
	108: 37,
	260: 38,
	306: 39,
	447: 40,
	82:  41,
	438: 42,
	227: 43,
	95:  44,
	164: 45,
	109: 46,
	394: 47,
	391: 48,
	213: 49,
	128: 50,
	182: 51,
	321: 52,
	110: 53,
	356: 54,
	440: 55,
	60:  56,
	75:  57,
	344: 58,
	81:  59,
	151: 60,
	303: 61,
	454: 62,
	323: 63,
	459: 64,
	98:  65,
	371: 66,
	123: 67,
	40:  68,
	439: 69,
	302: 70,
	58:  71,
	113: 72,
	496: 73,
	481: 74,
	229: 75,
	333: 76,
	443: 77,
	311: 78,
	418: 79,
	465: 80,
	378: 81,
	315: 82,
	445: 83,
	263: 84,
	270: 85,
	244: 86,
	42:  87,
	318: 88,
	13:  89,
	223: 90,
	28:  91,
	360: 92,
	379: 93,
	369: 94,
	434: 95,
	275: 96,
	22:  97,
	299: 98,
	238: 99,
	127: 100,
	21:  101,
	479: 102,
	467: 103,
	78:  104,
	101: 105,
	198: 106,
	313: 107,
	416: 108,
	466: 109,
	358: 110,
	96:  111,
	385: 112,
	487: 113,
	45:  114,
	52:  115,
	135: 116,
	234: 117,
	368: 118,
	33:  119,
	414: 120,
	328: 121,
	19:  122,
	100: 123,
	142: 124,
	210: 125,
	37:  126,
	31:  127,
	410: 128,
	206: 129,
	3:   130,
	497: 131,
	431: 132,
	258: 133,
	334: 134,
	243: 135,
	366: 136,
	71:  137,
	377: 138,
	417: 139,
	365: 140,
	166: 141,
	205: 142,
	4:   143,
	289: 144,
	305: 145,
	250: 146,
	48:  147,
	230: 148,
	455: 149,
	372: 150,
	373: 151,
	8:   152,
	94:  153,
	433: 154,
	192: 155,
	148: 156,
	274: 157,
	77:  158,
	422: 159,
	301: 160,
	376: 161,
	489: 162,
	461: 163,
	257: 164,
	290: 165,
	267: 166,
	364: 167,
	72:  168,
	354: 169,
	15:  170,
	69:  171,
	36:  172,
	235: 173,
	297: 174,
	185: 175,
	331: 176,
	390: 177,
	424: 178,
	411: 179,
	74:  180,
	248: 181,
	131: 182,
	59:  183,
	287: 184,
	132: 185,
	43:  186,
	430: 187,
	212: 188,
	247: 189,
	409: 190,
	265: 191,
	84:  192,
	180: 193,
	345: 194,
	475: 195,
	90:  196,
	157: 197,
	427: 198,
	340: 199,
	419: 200,
	6:   201,
	404: 202,
	298: 203,
	493: 204,
	224: 205,
	249: 206,
	495: 207,
	140: 208,
	152: 209,
	329: 210,
	35:  211,
	359: 212,
	351: 213,
	432: 214,
	9:   215,
	335: 216,
	167: 217,
	7:   218,
	350: 219,
	11:  220,
	346: 221,
	476: 222,
	220: 223,
	171: 224,
	436: 225,
	262: 226,
	240: 227,
	195: 228,
	125: 229,
	453: 230,
	112: 231,
	279: 232,
	200: 233,
	209: 234,
	500: 235,
	204: 236,
	320: 237,
	402: 238,
	261: 239,
	46:  240,
	392: 241,
	437: 242,
	38:  243,
	190: 244,
	435: 245,
	271: 246,
	468: 247,
	92:  248,
	231: 249,
	363: 250,
	251: 251,
	179: 252,
	246: 253,
	347: 254,
	162: 255,
	498: 256,
	412: 257,
	237: 258,
	382: 259,
	86:  260,
	317: 261,
	191: 262,
	425: 263,
	370: 264,
	41:  265,
	105: 266,
	450: 267,
	170: 268,
	471: 269,
	114: 270,
	65:  271,
	174: 272,
	211: 273,
	269: 274,
	62:  275,
	280: 276,
	442: 277,
	93:  278,
	446: 279,
	20:  280,
	49:  281,
	122: 282,
	2:   283,
	68:  284,
	115: 285,
	188: 286,
	474: 287,
	304: 288,
	187: 289,
	327: 290,
	316: 291,
	225: 292,
	17:  293,
	161: 294,
	111: 295,
	292: 296,
	194: 297,
	338: 298,
	406: 299,
	57:  300,
	388: 301,
	272: 302,
	499: 303,
	253: 304,
	393: 305,
	285: 306,
	61:  307,
	181: 308,
	282: 309,
	107: 310,
	136: 311,
	494: 312,
	362: 313,
	12:  314,
	286: 315,
	463: 316,
	147: 317,
	80:  318,
	444: 319,
	160: 320,
	254: 321,
	295: 322,
	426: 323,
	462: 324,
	233: 325,
	423: 326,
	186: 327,
	104: 328,
	284: 329,
	156: 330,
	5:   331,
	407: 332,
	196: 333,
	219: 334,
	488: 335,
	239: 336,
	159: 337,
	25:  338,
	143: 339,
	399: 340,
	252: 341,
	144: 342,
	63:  343,
	264: 344,
	387: 345,
	477: 346,
	87:  347,
	491: 348,
	215: 349,
	85:  350,
	149: 351,
	458: 352,
	228: 353,
	314: 354,
	464: 355,
	175: 356,
	97:  357,
	389: 358,
	88:  359,
	441: 360,
	121: 361,
	137: 362,
	130: 363,
	146: 364,
	383: 365,
	23:  366,
	349: 367,
	266: 368,
	255: 369,
	451: 370,
	483: 371,
	325: 372,
	256: 373,
	73:  374,
	177: 375,
	241: 376,
	14:  377,
	405: 378,
	1:   379,
	342: 380,
	154: 381,
	291: 382,
	197: 383,
	176: 384,
	54:  385,
	374: 386,
	312: 387,
	29:  388,
	203: 389,
	281: 390,
	118: 391,
	79:  392,
	50:  393,
	150: 394,
	415: 395,
	27:  396,
	268: 397,
	288: 398,
	352: 399,
	32:  400,
	473: 401,
	339: 402,
	341: 403,
	44:  404,
	357: 405,
	310: 406,
	53:  407,
	278: 408,
	367: 409,
	485: 410,
	309: 411,
	165: 412,
	199: 413,
	120: 414,
	380: 415,
	403: 416,
	276: 417,
	400: 418,
	384: 419,
	116: 420,
	375: 421,
	396: 422,
	484: 423,
	294: 424,
	480: 425,
	348: 426,
	355: 427,
	353: 428,
	106: 429,
	478: 430,
	47:  431,
	91:  432,
	16:  433,
	337: 434,
	155: 435,
	226: 436,
	322: 437,
	308: 438,
	492: 439,
	456: 440,
	30:  441,
	168: 442,
	319: 443,
	307: 444,
	217: 445,
	408: 446,
	293: 447,
	55:  448,
	273: 449,
	242: 450,
	398: 451,
	126: 452,
	277: 453,
	486: 454,
	472: 455,
	173: 456,
	448: 457,
	381: 458,
	76:  459,
	214: 460,
	296: 461,
	395: 462,
	386: 463,
	324: 464,
	401: 465,
	134: 466,
	449: 467,
	184: 468,
	10:  469,
	218: 470,
	428: 471,
	361: 472,
	208: 473,
	141: 474,
	201: 475,
	39:  476,
	153: 477,
	343: 478,
	83:  479,
	460: 480,
	99:  481,
	67:  482,
	429: 483,
	138: 484,
	330: 485,
	413: 486,
	172: 487,
	139: 488,
	326: 489,
	202: 490,
	189: 491,
	145: 492,
	232: 493,
	207: 494,
	24:  495,
	66:  496,
	18:  497,
	124: 498,
	64:  499,
}

// Figure31Integration manages the Figure31 site event.
type Figure31Integration struct {
	UserID         persist.DBID
	CollectionID   persist.DBID
	ContractAddr   common.Address
	ArtistAddr     common.Address
	ColumnCount    int
	CollectionSize int

	logs chan types.Log
	l    *dataloader.Loaders
	p    *multichain.Provider
	r    *persist.Repositories
	q    *pgxpool.Pool
	e    *ethclient.Client
}

// Figure31IntegrationInput contains the input params to configure the Figure31 integration.
type Figure31IntegrationInput struct {
	UserID         persist.DBID
	CollectionID   persist.DBID
	ContractAddr   string
	ArtistAddr     string
	ColumnCount    int
	CollectionSize int
}

// NewFigure31Integration returns a new Figure31 site integration
func NewFigure31Integration(loaders *dataloader.Loaders, provider *multichain.Provider, repos *persist.Repositories, pgx *pgxpool.Pool, input Figure31IntegrationInput) *Figure31Integration {
	ethClient, err := ethclient.Dial(viper.GetString("CONTRACT_INTERACTION_URL"))

	if err != nil {
		panic(err)
	}

	return &Figure31Integration{
		UserID:         input.UserID,
		CollectionID:   input.CollectionID,
		ContractAddr:   common.HexToAddress(input.ContractAddr),
		ArtistAddr:     common.HexToAddress(input.ArtistAddr),
		ColumnCount:    input.ColumnCount,
		CollectionSize: input.CollectionSize,
		logs:           make(chan types.Log),
		l:              loaders,
		p:              provider,
		r:              repos,
		q:              pgx,
		e:              ethClient,
	}
}

// Start listens for transfer events from the project's contracts and syncs the target collection.
func (i *Figure31Integration) Start(ctx context.Context) {
	logger.For(ctx).Info("starting Figure31 integration")

	for {
		<-time.After(3 * time.Minute)

		err := i.SyncCollection(ctx)
		if err != nil {
			logger.For(ctx).Error(err)
			sentryutil.ReportError(ctx, err)
		}

		err = i.AddToEarlyAccess(ctx)
		if err != nil {
			logger.For(ctx).Error(err)
			sentryutil.ReportError(ctx, err)
		}
	}
}

// SyncCollection syncs the user's wallet, and updates the collection.
func (i *Figure31Integration) SyncCollection(ctx context.Context) error {
	err := i.p.SyncTokens(ctx, i.UserID)
	if err != nil {
		return err
	}

	tokens, err := i.l.TokensByCollectionID.Load(i.CollectionID)
	if err != nil {
		return err
	}

	tokenMap := make([]persist.DBID, i.CollectionSize)
	for _, token := range tokens {
		mintID, err := strconv.ParseInt(token.TokenID.String, 16, 32)
		if err != nil {
			return err
		}
		pos, ok := tokenPositions[int(mintID)]
		if !ok {
			return fmt.Errorf("mintID=%d was not placed in collection", mintID)
		}
		tokenMap[pos] = token.ID
	}

	collectionTokens := make([]persist.DBID, 0)
	whitespace := make([]int, 0)
	transferPtr := 0

	for _, tokenID := range tokenMap {
		switch tokenID {
		case "":
			whitespace = append(whitespace, transferPtr)
		default:
			collectionTokens = append(collectionTokens, tokenID)
			transferPtr++
		}
	}

	return i.r.CollectionRepository.UpdateTokens(ctx, i.CollectionID, i.UserID, persist.CollectionUpdateTokensInput{
		LastUpdated: persist.LastUpdatedTime(time.Now()),
		Tokens:      collectionTokens,
		Layout:      persist.TokenLayout{Columns: persist.NullInt32(i.ColumnCount), Whitespace: whitespace},
	})
}

// AddToEarlyAccess only adds addresses that received tokens transferred from the artist's wallet.
// Every wallet is added each time in case an event was missed when the server wasn't available.
func (i *Figure31Integration) AddToEarlyAccess(ctx context.Context) error {
	query := ethereum.FilterQuery{Addresses: []common.Address{i.ContractAddr}, Topics: [][]common.Hash{
		{common.HexToHash(transferHash)}, {i.ArtistAddr.Hash()}},
	}

	logs, err := i.e.FilterLogs(ctx, query)
	if err != nil {
		return err
	}

	addresses := make([]string, 0)

	for _, log := range logs {
		if !log.Removed {
			toAddr := common.HexToAddress(log.Topics[2].Hex())
			addresses = append(addresses, strings.ToLower(toAddr.Hex()))
		}
	}

	if len(addresses) > 0 {
		insertQuery := "INSERT INTO early_access (address) SELECT unnest($1::TEXT[]) ON CONFLICT DO NOTHING;"
		_, err = i.q.Exec(ctx, insertQuery, pq.Array(addresses))
		if err != nil {
			return err
		}
	}

	return nil
}
