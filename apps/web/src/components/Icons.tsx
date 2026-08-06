import type React from 'react';

export const EthIcon: React.FC<{ className?: string }> = ({
	className = 'w-4 h-4',
}) => (
	<svg
		className={className}
		viewBox="0 0 784 1277"
		fill="none"
		xmlns="http://www.w3.org/2000/svg"
	>
		<title>Ethereum Logo</title>
		<path
			d="M392 0L383.5 28.8V872.2L392 880.7L784 649.3L392 0Z"
			fill="#627EEA"
		/>
		<path d="M392 0L0 649.3L392 880.7V470.9V0Z" fill="#8A9D8F" />
		<path
			d="M392 956L386.8 962.3V1268.4L392 1277L784 725.1L392 956Z"
			fill="#627EEA"
		/>
		<path d="M392 1277V956L0 725.1L392 1277Z" fill="#8A9D8F" />
		<path d="M392 880.7L784 649.3L392 470.9V880.7Z" fill="#454A75" />
		<path d="M0 649.3L392 880.7V470.9L0 649.3Z" fill="#5A6665" />
	</svg>
);

export const UsdtIcon: React.FC<{ className?: string }> = ({
	className = 'w-4 h-4',
}) => (
	<svg
		className={className}
		viewBox="0 0 2000 2000"
		fill="none"
		xmlns="http://www.w3.org/2000/svg"
	>
		<title>Tether USDT Logo</title>
		<path
			d="M1000 2000C1552.28 2000 2000 1552.28 2000 1000C2000 447.715 1552.28 0 1000 0C447.715 0 0 447.715 0 1000C0 1552.28 447.715 2000 1000 2000Z"
			fill="#26A17B"
		/>
		<path
			d="M1275 675H1600V425H400V675H725V950C540 965 400 1025 400 1100C400 1175 540 1235 725 1250V1575H1275V1250C1460 1235 1600 1175 1600 1100C1600 1025 1460 965 1275 950V675ZM1000 1175C735 1175 580 1135 580 1100C580 1065 735 1025 1000 1025C1265 1025 1420 1065 1420 1100C1420 1135 1265 1175 1000 1175Z"
			fill="white"
		/>
	</svg>
);

export const UsdcIcon: React.FC<{ className?: string }> = ({
	className = 'w-4 h-4',
}) => (
	<svg
		className={className}
		viewBox="0 0 2000 2000"
		fill="none"
		xmlns="http://www.w3.org/2000/svg"
	>
		<title>USD Coin Logo</title>
		<circle cx="1000" cy="1000" r="1000" fill="#2775CA" />
		<path
			d="M1000 400C668.6 400 400 668.6 400 1000C400 1331.4 668.6 1600 1000 1600C1331.4 1600 1600 1331.4 1600 1000C1600 668.6 1331.4 400 1000 400ZM1000 1450C751.5 1450 550 1248.5 550 1000C550 751.5 751.5 550 1000 550C1248.5 550 1450 751.5 1450 1000C1450 1248.5 1248.5 1450 1000 1450Z"
			fill="white"
		/>
		<path
			d="M925 700H1075V850H925V700ZM925 1150H1075V1300H925V1150ZM800 875H1125V1000H800V875ZM875 1000H1200V1125H875V1000Z"
			fill="white"
		/>
	</svg>
);

export const BtcIcon: React.FC<{ className?: string }> = ({
	className = 'w-4 h-4',
}) => (
	<svg
		className={className}
		viewBox="0 0 2000 2000"
		fill="none"
		xmlns="http://www.w3.org/2000/svg"
	>
		<title>Bitcoin Logo</title>
		<circle cx="1000" cy="1000" r="1000" fill="#F7931A" />
		<path
			d="M1411 818C1434 664 1337 580 1184 527L1228 350L1120 323L1077 496C1048 489 1018 482 988 475L1031 301L924 274L880 447L648 389L604 566C604 566 690 585 688 587C735 599 744 631 737 657L660 967C654 981 640 1001 607 993C609 996 523 974 523 974L454 1133L673 1188C703 1196 732 1204 761 1211L717 1387L824 1414L868 1241C898 1249 927 1256 955 1263L911 1439L1019 1466L1063 1293C1248 1328 1387 1313 1438 1146C1479 1011 1428 933 1331 878C1402 862 1455 813 1411 818ZM1240 1111C1204 1255 964 1186 886 1167L936 967C1014 986 1277 1012 1240 1111ZM1276 817C1243 949 1044 887 979 871L1025 688C1090 704 1310 726 1276 817Z"
			fill="white"
		/>
	</svg>
);
