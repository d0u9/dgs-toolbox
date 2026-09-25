package country

// table is ISO 3166-1, one country per line: alpha-2, alpha-3, the English
// short name, the Chinese short name, then other names it is written as,
// separated by ";". Matching ignores case, spaces, dots and the word "the".
const table = `
AD|AND|Andorra|安道尔|
AE|ARE|United Arab Emirates|阿联酋|UAE;阿拉伯联合酋长国
AF|AFG|Afghanistan|阿富汗|
AG|ATG|Antigua and Barbuda|安提瓜和巴布达|
AI|AIA|Anguilla|安圭拉|
AL|ALB|Albania|阿尔巴尼亚|
AM|ARM|Armenia|亚美尼亚|
AO|AGO|Angola|安哥拉|
AQ|ATA|Antarctica|南极洲|
AR|ARG|Argentina|阿根廷|
AS|ASM|American Samoa|美属萨摩亚|
AT|AUT|Austria|奥地利|
AU|AUS|Australia|澳大利亚|澳洲;Commonwealth of Australia
AW|ABW|Aruba|阿鲁巴|
AX|ALA|Åland Islands|奥兰群岛|Aland Islands
AZ|AZE|Azerbaijan|阿塞拜疆|
BA|BIH|Bosnia and Herzegovina|波黑|波斯尼亚和黑塞哥维那
BB|BRB|Barbados|巴巴多斯|
BD|BGD|Bangladesh|孟加拉国|孟加拉
BE|BEL|Belgium|比利时|
BF|BFA|Burkina Faso|布基纳法索|
BG|BGR|Bulgaria|保加利亚|
BH|BHR|Bahrain|巴林|
BI|BDI|Burundi|布隆迪|
BJ|BEN|Benin|贝宁|
BL|BLM|Saint Barthélemy|圣巴泰勒米|Saint Barthelemy
BM|BMU|Bermuda|百慕大|
BN|BRN|Brunei|文莱|Brunei Darussalam
BO|BOL|Bolivia|玻利维亚|
BQ|BES|Caribbean Netherlands|荷兰加勒比区|Bonaire, Sint Eustatius and Saba
BR|BRA|Brazil|巴西|
BS|BHS|Bahamas|巴哈马|
BT|BTN|Bhutan|不丹|
BV|BVT|Bouvet Island|布韦岛|
BW|BWA|Botswana|博茨瓦纳|
BY|BLR|Belarus|白俄罗斯|
BZ|BLZ|Belize|伯利兹|
CA|CAN|Canada|加拿大|
CC|CCK|Cocos (Keeling) Islands|科科斯（基林）群岛|Cocos Islands
CD|COD|DR Congo|刚果（金）|Democratic Republic of the Congo;刚果民主共和国
CF|CAF|Central African Republic|中非|中非共和国
CG|COG|Congo|刚果（布）|Republic of the Congo;刚果共和国
CH|CHE|Switzerland|瑞士|
CI|CIV|Côte d'Ivoire|科特迪瓦|Cote d'Ivoire;Ivory Coast
CK|COK|Cook Islands|库克群岛|
CL|CHL|Chile|智利|
CM|CMR|Cameroon|喀麦隆|
CN|CHN|China|中国|中华人民共和国;People's Republic of China;PRC;P.R.China;P.R.C.;中國;中华;China (Mainland)
CO|COL|Colombia|哥伦比亚|
CR|CRI|Costa Rica|哥斯达黎加|
CU|CUB|Cuba|古巴|
CV|CPV|Cabo Verde|佛得角|Cape Verde
CW|CUW|Curaçao|库拉索|Curacao
CX|CXR|Christmas Island|圣诞岛|
CY|CYP|Cyprus|塞浦路斯|
CZ|CZE|Czechia|捷克|Czech Republic
DE|DEU|Germany|德国|Deutschland
DJ|DJI|Djibouti|吉布提|
DK|DNK|Denmark|丹麦|
DM|DMA|Dominica|多米尼克|
DO|DOM|Dominican Republic|多米尼加|多米尼加共和国
DZ|DZA|Algeria|阿尔及利亚|
EC|ECU|Ecuador|厄瓜多尔|
EE|EST|Estonia|爱沙尼亚|
EG|EGY|Egypt|埃及|
EH|ESH|Western Sahara|西撒哈拉|
ER|ERI|Eritrea|厄立特里亚|
ES|ESP|Spain|西班牙|España
ET|ETH|Ethiopia|埃塞俄比亚|
FI|FIN|Finland|芬兰|
FJ|FJI|Fiji|斐济|
FK|FLK|Falkland Islands|福克兰群岛|
FM|FSM|Micronesia|密克罗尼西亚|
FO|FRO|Faroe Islands|法罗群岛|
FR|FRA|France|法国|
GA|GAB|Gabon|加蓬|
GB|GBR|United Kingdom|英国|UK;U.K.;Great Britain;Britain;England;United Kingdom of Great Britain and Northern Ireland
GD|GRD|Grenada|格林纳达|
GE|GEO|Georgia|格鲁吉亚|
GF|GUF|French Guiana|法属圭亚那|
GG|GGY|Guernsey|根西|
GH|GHA|Ghana|加纳|
GI|GIB|Gibraltar|直布罗陀|
GL|GRL|Greenland|格陵兰|
GM|GMB|Gambia|冈比亚|
GN|GIN|Guinea|几内亚|
GP|GLP|Guadeloupe|瓜德罗普|
GQ|GNQ|Equatorial Guinea|赤道几内亚|
GR|GRC|Greece|希腊|
GS|SGS|South Georgia and the South Sandwich Islands|南乔治亚和南桑威奇群岛|
GT|GTM|Guatemala|危地马拉|
GU|GUM|Guam|关岛|
GW|GNB|Guinea-Bissau|几内亚比绍|
GY|GUY|Guyana|圭亚那|
HK|HKG|Hong Kong|中国香港|香港;Hong Kong SAR;HKSAR
HM|HMD|Heard Island and McDonald Islands|赫德岛和麦克唐纳群岛|
HN|HND|Honduras|洪都拉斯|
HR|HRV|Croatia|克罗地亚|
HT|HTI|Haiti|海地|
HU|HUN|Hungary|匈牙利|
ID|IDN|Indonesia|印度尼西亚|印尼
IE|IRL|Ireland|爱尔兰|
IL|ISR|Israel|以色列|
IM|IMN|Isle of Man|马恩岛|
IN|IND|India|印度|
IO|IOT|British Indian Ocean Territory|英属印度洋领地|
IQ|IRQ|Iraq|伊拉克|
IR|IRN|Iran|伊朗|
IS|ISL|Iceland|冰岛|
IT|ITA|Italy|意大利|
JE|JEY|Jersey|泽西|
JM|JAM|Jamaica|牙买加|
JO|JOR|Jordan|约旦|
JP|JPN|Japan|日本|
KE|KEN|Kenya|肯尼亚|
KG|KGZ|Kyrgyzstan|吉尔吉斯斯坦|
KH|KHM|Cambodia|柬埔寨|
KI|KIR|Kiribati|基里巴斯|
KM|COM|Comoros|科摩罗|
KN|KNA|Saint Kitts and Nevis|圣基茨和尼维斯|
KP|PRK|North Korea|朝鲜|DPRK
KR|KOR|South Korea|韩国|Korea;Republic of Korea;大韩民国
KW|KWT|Kuwait|科威特|
KY|CYM|Cayman Islands|开曼群岛|
KZ|KAZ|Kazakhstan|哈萨克斯坦|
LA|LAO|Laos|老挝|
LB|LBN|Lebanon|黎巴嫩|
LC|LCA|Saint Lucia|圣卢西亚|
LI|LIE|Liechtenstein|列支敦士登|
LK|LKA|Sri Lanka|斯里兰卡|
LR|LBR|Liberia|利比里亚|
LS|LSO|Lesotho|莱索托|
LT|LTU|Lithuania|立陶宛|
LU|LUX|Luxembourg|卢森堡|
LV|LVA|Latvia|拉脱维亚|
LY|LBY|Libya|利比亚|
MA|MAR|Morocco|摩洛哥|
MC|MCO|Monaco|摩纳哥|
MD|MDA|Moldova|摩尔多瓦|
ME|MNE|Montenegro|黑山|
MF|MAF|Saint Martin|法属圣马丁|
MG|MDG|Madagascar|马达加斯加|
MH|MHL|Marshall Islands|马绍尔群岛|
MK|MKD|North Macedonia|北马其顿|Macedonia
ML|MLI|Mali|马里|
MM|MMR|Myanmar|缅甸|Burma
MN|MNG|Mongolia|蒙古|蒙古国
MO|MAC|Macao|中国澳门|澳门;Macau;Macao SAR
MP|MNP|Northern Mariana Islands|北马里亚纳群岛|
MQ|MTQ|Martinique|马提尼克|
MR|MRT|Mauritania|毛里塔尼亚|
MS|MSR|Montserrat|蒙特塞拉特|
MT|MLT|Malta|马耳他|
MU|MUS|Mauritius|毛里求斯|
MV|MDV|Maldives|马尔代夫|
MW|MWI|Malawi|马拉维|
MX|MEX|Mexico|墨西哥|
MY|MYS|Malaysia|马来西亚|
MZ|MOZ|Mozambique|莫桑比克|
NA|NAM|Namibia|纳米比亚|
NC|NCL|New Caledonia|新喀里多尼亚|
NE|NER|Niger|尼日尔|
NF|NFK|Norfolk Island|诺福克岛|
NG|NGA|Nigeria|尼日利亚|
NI|NIC|Nicaragua|尼加拉瓜|
NL|NLD|Netherlands|荷兰|Holland
NO|NOR|Norway|挪威|
NP|NPL|Nepal|尼泊尔|
NR|NRU|Nauru|瑙鲁|
NU|NIU|Niue|纽埃|
NZ|NZL|New Zealand|新西兰|
OM|OMN|Oman|阿曼|
PA|PAN|Panama|巴拿马|
PE|PER|Peru|秘鲁|
PF|PYF|French Polynesia|法属波利尼西亚|
PG|PNG|Papua New Guinea|巴布亚新几内亚|
PH|PHL|Philippines|菲律宾|
PK|PAK|Pakistan|巴基斯坦|
PL|POL|Poland|波兰|
PM|SPM|Saint Pierre and Miquelon|圣皮埃尔和密克隆|
PN|PCN|Pitcairn Islands|皮特凯恩群岛|
PR|PRI|Puerto Rico|波多黎各|
PS|PSE|Palestine|巴勒斯坦|
PT|PRT|Portugal|葡萄牙|
PW|PLW|Palau|帕劳|
PY|PRY|Paraguay|巴拉圭|
QA|QAT|Qatar|卡塔尔|
RE|REU|Réunion|留尼汪|Reunion
RO|ROU|Romania|罗马尼亚|
RS|SRB|Serbia|塞尔维亚|
RU|RUS|Russia|俄罗斯|Russian Federation;俄罗斯联邦
RW|RWA|Rwanda|卢旺达|
SA|SAU|Saudi Arabia|沙特阿拉伯|沙特
SB|SLB|Solomon Islands|所罗门群岛|
SC|SYC|Seychelles|塞舌尔|
SD|SDN|Sudan|苏丹|
SE|SWE|Sweden|瑞典|
SG|SGP|Singapore|新加坡|
SH|SHN|Saint Helena|圣赫勒拿|
SI|SVN|Slovenia|斯洛文尼亚|
SJ|SJM|Svalbard and Jan Mayen|斯瓦尔巴和扬马延|
SK|SVK|Slovakia|斯洛伐克|
SL|SLE|Sierra Leone|塞拉利昂|
SM|SMR|San Marino|圣马力诺|
SN|SEN|Senegal|塞内加尔|
SO|SOM|Somalia|索马里|
SR|SUR|Suriname|苏里南|
SS|SSD|South Sudan|南苏丹|
ST|STP|São Tomé and Príncipe|圣多美和普林西比|Sao Tome and Principe
SV|SLV|El Salvador|萨尔瓦多|
SX|SXM|Sint Maarten|荷属圣马丁|
SY|SYR|Syria|叙利亚|
SZ|SWZ|Eswatini|斯威士兰|Swaziland
TC|TCA|Turks and Caicos Islands|特克斯和凯科斯群岛|
TD|TCD|Chad|乍得|
TF|ATF|French Southern Territories|法属南部领地|
TG|TGO|Togo|多哥|
TH|THA|Thailand|泰国|
TJ|TJK|Tajikistan|塔吉克斯坦|
TK|TKL|Tokelau|托克劳|
TL|TLS|Timor-Leste|东帝汶|East Timor
TM|TKM|Turkmenistan|土库曼斯坦|
TN|TUN|Tunisia|突尼斯|
TO|TON|Tonga|汤加|
TR|TUR|Türkiye|土耳其|Turkey
TT|TTO|Trinidad and Tobago|特立尼达和多巴哥|
TV|TUV|Tuvalu|图瓦卢|
TW|TWN|Taiwan|中国台湾|台湾
TZ|TZA|Tanzania|坦桑尼亚|
UA|UKR|Ukraine|乌克兰|
UG|UGA|Uganda|乌干达|
UM|UMI|United States Minor Outlying Islands|美国本土外小岛屿|
US|USA|United States|美国|USA;U.S.A.;U.S.;America;United States of America;美利坚合众国
UY|URY|Uruguay|乌拉圭|
UZ|UZB|Uzbekistan|乌兹别克斯坦|
VA|VAT|Vatican City|梵蒂冈|Holy See
VC|VCT|Saint Vincent and the Grenadines|圣文森特和格林纳丁斯|
VE|VEN|Venezuela|委内瑞拉|
VG|VGB|British Virgin Islands|英属维尔京群岛|
VI|VIR|U.S. Virgin Islands|美属维尔京群岛|
VN|VNM|Vietnam|越南|Viet Nam
VU|VUT|Vanuatu|瓦努阿图|
WF|WLF|Wallis and Futuna|瓦利斯和富图纳|
WS|WSM|Samoa|萨摩亚|
YE|YEM|Yemen|也门|
YT|MYT|Mayotte|马约特|
ZA|ZAF|South Africa|南非|
ZM|ZMB|Zambia|赞比亚|
ZW|ZWE|Zimbabwe|津巴布韦|
`
