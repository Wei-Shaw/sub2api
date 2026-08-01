package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

func normalizeLoginAgreementMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "checkbox":
		return "checkbox"
	default:
		return defaultLoginAgreementMode
	}
}

func defaultLoginAgreementDocuments() []LoginAgreementDocument {
	return []LoginAgreementDocument{
		{
			ID:        "terms",
			Title:     "服务条款",
			ContentMD: `V-Claw 服务条款第一条 (目的)本条款旨在规定 V-Claw（以下简称“公司”）提供的 V-Claw 安装及 AI Token 销售服务（以下简称“服务”）的使用条件和程序。第二条 (安装服务及费用)用户必须安装 OPENCLAW AI 代理方可使用本服务。安装费用为 99 元 (CNY)，为系统优化及对接的一次性必要费用。第三条 (Token 销售及使用)公司按等级套餐销售用于 AI 模型的令牌 (Token)。购买的 Token 仅在指定平台内有效，不得转让给他人。第四条 (退款政策)安装费： 技术支持及系统设置开始后，安装费不予退还。Token： 仅在购买后 7 天内且完全未使用的状态下可申请退款；鉴于数字资产的特殊性，一旦开始使用，概不退款。第五条 (质量保证)公司自系统交付之日起提供为期 12 个月的功能错误及漏洞免费保修。因用户疏忽、擅自修改源代码或不可抗力导致的问题不在保修范围内。第六条 (用户义务与禁止非法行为)用户不得利用本服务从事任何违反所在地及使用地国家法律的非法活动。若公司判定用户将服务用于非法目的，有权立即限制其使用。第七条 (免责条款)公司对用户因滥用服务或违反法律法规而导致的任何民事或刑事损害不承担任何责任，所有法律后果由用户本人承担。`,
			TitleI18n: map[string]string{
				"zh": `服务条款`,
				"en": `Terms of Service`,
				"vi": `Điều khoản dịch vụ`,
				"ko": `서비스 약관`,
			},
			ContentMDI18n: map[string]string{
				"zh": `V-Claw 服务条款第一条 (目的)本条款旨在规定 V-Claw（以下简称“公司”）提供的 V-Claw 安装及 AI Token 销售服务（以下简称“服务”）的使用条件和程序。第二条 (安装服务及费用)用户必须安装 OPENCLAW AI 代理方可使用本服务。安装费用为 99 元 (CNY)，为系统优化及对接的一次性必要费用。第三条 (Token 销售及使用)公司按等级套餐销售用于 AI 模型的令牌 (Token)。购买的 Token 仅在指定平台内有效，不得转让给他人。第四条 (退款政策)安装费： 技术支持及系统设置开始后，安装费不予退还。Token： 仅在购买后 7 天内且完全未使用的状态下可申请退款；鉴于数字资产的特殊性，一旦开始使用，概不退款。第五条 (质量保证)公司自系统交付之日起提供为期 12 个月的功能错误及漏洞免费保修。因用户疏忽、擅自修改源代码或不可抗力导致的问题不在保修范围内。第六条 (用户义务与禁止非法行为)用户不得利用本服务从事任何违反所在地及使用地国家法律的非法活动。若公司判定用户将服务用于非法目的，有权立即限制其使用。第七条 (免责条款)公司对用户因滥用服务或违反法律法规而导致的任何民事或刑事损害不承担任何责任，所有法律后果由用户本人承担。`,
				"en": `V-Claw Terms of ServiceArticle 1 (Purpose)These Terms govern the conditions and procedures for using the V-Claw installation and AI Token sales service (the “Service”) provided by V-Claw (“Company”).Article 2 (Installation Service & Fee)Users must install the OPENCLAW AI Agent to utilize the Service.The installation fee is 99 CNY, a one-time mandatory fee for system optimization and integration.Article 3 (Token Sales & Usage)The Company sells data tokens in tiered packages for AI model utilization.Purchased tokens are valid only within the designated platform and are non-transferable.Article 4 (Refund Policy)Setup Fee: Non-refundable once technical support or system configuration has commenced.Tokens: Refundable within 7 days only if entirely unused. Due to the nature of digital assets, no refunds will be issued once any portion of the tokens is used.Article 5 (Quality Assurance/Warranty)The Company provides a 12-month warranty for functional errors and bugs from the date of handover.Issues caused by user negligence, unauthorized source code modification, or force majeure are excluded.Article 6 (User Obligations & Illegal Use)Users shall not use the Service for any illegal activities violating the laws of their jurisdiction or international regulations.The Company reserves the right to terminate access if illegal use is suspected.Article 7 (Limitation of Liability)The Company is exempt from any liability for civil or criminal damages arising from the user's misuse of the Service. All legal consequences reside solely with the user.`,
				"vi": `Điều khoản Dịch vụ V-Claw
Điều 1 (Mục đích) Điều khoản này quy định các điều kiện và thủ tục sử dụng dịch vụ cài đặt V-Claw và bán Token AI (sau đây gọi là "Dịch vụ") do V-Claw(sau đây gọi là "Công ty") cung cấp.
Điều 2 (Dịch vụ cài đặt và Phí)
Người dùng bắt buộc phải cài đặt OPENCLAW AI Agent để sử dụng dịch vụ.
Phí cài đặt là 99 CNY, đây là chi phí cố định một lần duy nhất để tối ưu hóa và kết nối hệ thống.
Điều 3 (Bán và sử dụng Token)
Công ty bán các gói Token dữ liệu theo từng cấp độ để sử dụng các mô hình AI.
Token đã mua chỉ có giá trị trong nền tảng được chỉ định và không thể chuyển nhượng cho người khác.
Điều 4 (Chính sách hoàn tiền)
Phí cài đặt: Không hoàn lại sau khi quá trình hỗ trợ kỹ thuật và thiết lập hệ thống đã bắt đầu.
Token: Chỉ có thể hoàn tiền trong vòng 7 ngày nếu chưa sử dụng bất kỳ Token nào. Do tính chất của tài sản kỹ thuật số, sẽ không hoàn tiền nếu Token đã được sử dụng một phần hoặc toàn bộ.
Điều 5 (Bảo hành chất lượng)
Công ty cung cấp chế độ bảo hành miễn phí đối với các lỗi chức năng và lỗi phần mềm trong vòng 12 tháng kể từ ngày bàn giao hệ thống.
Các vấn đề phát sinh do sự bất cẩn của người dùng, tự ý sửa đổi mã nguồn hoặc bất khả kháng sẽ bị loại trừ khỏi phạm vi bảo hành.
Điều 6 (Nghĩa vụ người dùng và Nghiêm cấm hành vi vi phạm pháp luật)
Người dùng không được sử dụng dịch vụ để thực hiện bất kỳ hành vi bất hợp pháp nào vi phạm pháp luật của quốc gia sở tại và quốc gia sử dụng.
Công ty có quyền hạn chế sử dụng dịch vụ ngay lập tức nếu xác định người dùng sử dụng dịch vụ cho mục đích bất hợp pháp.
Điều 7 (Miễn trừ trách nhiệm) Công ty miễn trừ mọi trách nhiệm đối với các thiệt hại dân sự hoặc hình sự phát sinh do việc người dùng lạm dụng dịch vụ hoặc vi phạm quy định pháp luật. Mọi trách nhiệm pháp lý thuộc về cá nhân người dùng.`,
				"ko": `V-Claw 서비스 이용약관
제1조 (목적) 본 약관은 V-Claw(이하 "회사")가 제공하는 V-Claw 설치 및 AI 토큰 판매 서비스(이하 "서비스")의 이용 조건과 절차에 관한 사항을 규정함을 목적으로 합니다.
제2조 (설치 서비스 및 비용)
사용자는 서비스 이용을 위해 OPENCLAW AI 에이전트를 필수적으로 설치해야 합니다.
설치 비용은 99 CNY(위안)이며, 이는 시스템 최적화 및 연동을 위한 최초 1회 필수 비용입니다.
제3조 (토큰 판매 및 사용)
회사는 AI 모델 활용을 위한 데이터 토큰을 등급별 패키지로 판매합니다.
구매한 토큰은 회사가 지정한 플랫폼 내에서만 유효하며, 타인에게 양도할 수 없습니다.
제4조 (환불 정책)
설치비: 기술 지원 및 시스템 세팅이 시작된 이후에는 설치비 환불이 불가합니다.
토큰: 구매 후 토큰을 전혀 사용하지 않은 경우에 한해 7일 이내 환불이 가능하며, 일부라도 사용된 경우 디지털 자산의 특성상 환불이 불가합니다.
제5조 (품질 보증)
회사는 시스템 인도일로부터 12개월간 기능적 오류 및 버그에 대해 무상 보증을 제공합니다.
단, 사용자의 부주의, 소스 코드 임의 수정, 천재지변으로 인한 문제는 보증 범위에서 제외됩니다.
제6조 (사용자 의무 및 불법 행위 금지)
사용자는 서비스를 이용하여 거주 국가 및 이용 대상 국가의 법률을 위반하는 어떠한 불법 행위도 해서는 안 됩니다.
회사는 사용자가 서비스를 불법적인 목적으로 이용한다고 판단될 경우 즉시 서비스 이용을 제한할 수 있습니다.
제7조 (면책 조항) 회사는 사용자의 서비스 오용이나 법적 규제 위반으로 인해 발생한 어떠한 민·형사상 손해에 대해서도 책임을 지지 않으며, 모든 법적 책임은 사용자 본인에게 있습니다.`,
			},
		},
		{
			ID:    "usage-policy",
			Title: "使用政策",
			ContentMD: `本免责声明为法律公告，适用于所有使用 OPENCLAW 安装服务及 Token 销售服务（以下简称“本服务”）的用户。
严禁非法使用： 用户不得利用本服务从事任何违反其居住国、使用地国家法律或国际法的非法活动（包括但不限于黑客攻击、欺诈、非法赌博、侵犯版权、盗用个人信息、生成有害内容等）。
用户的绝对责任： 用户应对通过本服务开展的所有活动、数据处理及其后果承担全部法律及道德责任。对于因用户滥用本服务或违反法律法规而导致的所有民事或刑事问题，提供方不承担任何责任。
提供方免责： 本服务提供方（以下简称“提供方”）没有义务监督用户的具体使用目的或活动内容。对于因用户的违法行为而导致的任何直接或间接损失、法律纠纷或行政处罚，提供方均免于承担任何法律责任。
服务限制与终止权： 若提供方判定用户将本服务用于非法目的，或其行为违反本声明之原则，提供方保留在不事先通知的情况下立即限制或终止其服务使用的权利。`,
			TitleI18n: map[string]string{
				"zh": `使用政策`,
				"en": `Usage Policy`,
				"vi": `Chính sách sử dụng`,
				"ko": `이용 정책`,
			},
			ContentMDI18n: map[string]string{
				"zh": `本免责声明为法律公告，适用于所有使用 OPENCLAW 安装服务及 Token 销售服务（以下简称“本服务”）的用户。
严禁非法使用： 用户不得利用本服务从事任何违反其居住国、使用地国家法律或国际法的非法活动（包括但不限于黑客攻击、欺诈、非法赌博、侵犯版权、盗用个人信息、生成有害内容等）。
用户的绝对责任： 用户应对通过本服务开展的所有活动、数据处理及其后果承担全部法律及道德责任。对于因用户滥用本服务或违反法律法规而导致的所有民事或刑事问题，提供方不承担任何责任。
提供方免责： 本服务提供方（以下简称“提供方”）没有义务监督用户的具体使用目的或活动内容。对于因用户的违法行为而导致的任何直接或间接损失、法律纠纷或行政处罚，提供方均免于承担任何法律责任。
服务限制与终止权： 若提供方判定用户将本服务用于非法目的，或其行为违反本声明之原则，提供方保留在不事先通知的情况下立即限制或终止其服务使用的权利。`,
				"en": `This Disclaimer is a legal notice applicable to all users of the OPENCLAW installation service and Token sales service (hereinafter referred to as "the Service").
Strict Prohibition of Illegal Use: Users shall not utilize the Service for any illegal activities that violate the laws of their jurisdiction, the laws of the country of use, or international regulations (including but not limited to hacking, fraud, illegal gambling, copyright infringement, identity theft, or generation of harmful content).
Sole Responsibility of the User: The user assumes full legal and moral responsibility for all activities, data processing, and consequences arising from the use of the Service. The Service Provider shall not be held liable for any civil or criminal issues resulting from the user's misuse of the Service or violation of legal statutes.
Limitation and Exemption of Liability: The Service Provider (hereinafter referred to as "the Provider") is under no obligation to monitor the specific intent or activities of individual users. The Provider shall be exempt from any and all liability regarding direct or indirect damages, legal disputes, or administrative sanctions arising from the user's unlawful conduct.
Restriction and Termination of Service: The Provider reserves the right to immediately restrict or terminate access to the Service without prior notice if it is determined, at the Provider's sole discretion, that the Service is being used for illegal purposes or in a manner inconsistent with this Disclaimer.`,
				"vi": `Tuyên bố này là thông báo pháp lý áp dụng cho tất cả người dùng sử dụng dịch vụ cài đặt OPENCLAW và dịch vụ bán Token (sau đây gọi là "Dịch vụ").
Nghiêm cấm tuyệt đối việc sử dụng trái phép: Người dùng không được sử dụng Dịch vụ để thực hiện bất kỳ hoạt động bất hợp pháp nào vi phạm pháp luật của quốc gia sở tại, quốc gia nơi Dịch vụ được sử dụng hoặc luật pháp quốc tế (bao gồm nhưng không giới hạn ở tấn công mạng, lừa đảo, cờ bạc bất hợp pháp, vi phạm bản quyền, đánh cắp thông tin cá nhân hoặc tạo nội dung độc hại).
Trách nhiệm duy nhất của người dùng: Người dùng chịu toàn bộ trách nhiệm pháp lý và đạo đức đối với mọi hoạt động, xử lý dữ liệu và hậu quả phát sinh từ việc sử dụng Dịch vụ. Nhà cung cấp sẽ không can thiệp hoặc chịu trách nhiệm cho bất kỳ vấn đề dân sự hay hình sự nào phát sinh từ việc người dùng lạm dụng Dịch vụ hoặc vi phạm quy định pháp luật.
Miễn trừ trách nhiệm cho nhà cung cấp: Nhà cung cấp dịch vụ (sau đây gọi là "Nhà cung cấp") không có nghĩa vụ giám sát mục đích sử dụng hoặc nội dung hoạt động cụ thể của từng người dùng. Nhà cung cấp được miễn trừ khỏi mọi trách nhiệm pháp lý đối với bất kỳ thiệt hại trực tiếp hay gián tiếp, tranh chấp pháp lý hoặc hình phạt hành chính nào phát sinh từ hành vi vi phạm pháp luật của người dùng.
Quyền hạn chế và chấm dứt dịch vụ: Nhà cung cấp có quyền hạn chế hoặc chặn quyền truy cập Dịch vụ ngay lập tức mà không cần thông báo trước nếu xác định rằng người dùng đang sử dụng Dịch vụ cho các mục đích bất hợp pháp hoặc có hành vi không phù hợp với tinh thần của Tuyên bố này.`,
				"ko": `면책 성명서
본 성명서는 OPENCLAW 설치 서비스 및 토큰 판매 서비스(이하 "본 서비스")를 이용하는 모든 사용자에게 적용되는 법적 고지입니다.
불법 이용의 엄격 금지: 사용자는 본 서비스를 이용하여 거주 국가, 이용 대상 국가 및 국제법을 위반하는 어떠한 불법적인 활동(해킹, 사기, 불법 도박, 저작권 침해, 개인정보 도용, 유해 콘텐츠 생성 등)도 수행해서는 안 됩니다.
사용자의 전적인 책임: 본 서비스를 통해 발생하는 모든 활동, 데이터 처리 및 그 결과에 대한 법적, 도덕적 책임은 전적으로 사용자 본인에게 있습니다. 사용자가 서비스를 오용하거나 법적 규제를 위반하여 발생하는 모든 민·형사상 문제에 대해 공급자는 개입하지 않습니다.
공급자의 면책: 본 서비스 제공자(이하 "공급자")는 사용자의 구체적인 이용 목적이나 활동 내용을 감시할 의무가 없습니다. 공급자는 사용자의 위법 행위로 인해 발생한 직·간접적인 손해, 법적 분쟁, 행정 처분 등에 대하여 어떠한 법적 책임도 지지 않으며, 모든 책임으로부터 면책됩니다.
서비스 이용의 제한 및 차단: 공급자는 사용자가 불법적인 목적으로 서비스를 이용하거나 본 성명의 취지에 어긋나는 행동을 한다고 판단될 경우, 사전 통지 없이 서비스 이용을 즉시 제한하거나 차단할 권리를 보유합니다.`,
			},
		},
		{
			ID:    "supported-regions",
			Title: "支持的国家和地区",
			ContentMD: `支持地区

本服务仅在法律法规、上游服务条款及当地合规要求允许的地区提供。用户应自行确认其居住国、使用地国家或地区允许访问和使用本服务。若相关地区法律、监管要求、网络环境或上游服务限制导致服务不可用、受限或中断，提供方可拒绝、暂停或终止相关服务。`,
			TitleI18n: map[string]string{
				"zh": `支持地区`,
				"en": `Supported Regions`,
				"vi": `Khu vực hỗ trợ`,
				"ko": `지원 지역`,
			},
			ContentMDI18n: map[string]string{
				"zh": `支持地区

本服务仅在法律法规、上游服务条款及当地合规要求允许的地区提供。用户应自行确认其居住国、使用地国家或地区允许访问和使用本服务。若相关地区法律、监管要求、网络环境或上游服务限制导致服务不可用、受限或中断，提供方可拒绝、暂停或终止相关服务。`,
				"en": `Supported Regions

This Service is available only in regions where applicable laws, upstream provider terms, and local compliance requirements permit its use. Users are responsible for confirming that access to and use of the Service are allowed in their country or region of residence and use. The provider may refuse, suspend, or terminate service where laws, regulations, network conditions, or upstream restrictions make the Service unavailable or restricted.`,
				"vi": `Khu vực hỗ trợ

Dịch vụ này chỉ được cung cấp tại những khu vực mà pháp luật hiện hành, điều khoản của nhà cung cấp thượng nguồn và yêu cầu tuân thủ địa phương cho phép. Người dùng có trách nhiệm tự xác nhận rằng quốc gia/khu vực cư trú và nơi sử dụng cho phép truy cập và sử dụng Dịch vụ. Nhà cung cấp có thể từ chối, tạm dừng hoặc chấm dứt dịch vụ nếu pháp luật, quy định, điều kiện mạng hoặc giới hạn của nhà cung cấp thượng nguồn khiến Dịch vụ không khả dụng hoặc bị hạn chế.`,
				"ko": `지원 지역

본 서비스는 관련 법령, 상위 제공업체 약관 및 현지 준수 요건이 허용하는 지역에서만 제공됩니다. 사용자는 거주 국가/지역 및 이용 지역에서 본 서비스 접근과 사용이 허용되는지 직접 확인해야 합니다. 법률, 규제, 네트워크 환경 또는 상위 서비스 제한으로 인해 서비스가 이용 불가하거나 제한되는 경우 제공자는 서비스를 거부, 일시 중지 또는 종료할 수 있습니다.`,
			},
		},
		{
			ID:    "service-specific-terms",
			Title: "特定服务条款",
			ContentMD: `服务特定条款

不同服务、模型、账号类型、套餐或 Token 产品可能适用额外限制、有效期、配额、退款规则、保修范围和上游平台条款。用户在购买、导入账号、生成 API Key 或使用具体模型前，应阅读对应页面、订单说明或管理员公告中的特定规则。若服务特定条款与通用条款不一致，以更严格或更具体的条款为准。`,
			TitleI18n: map[string]string{
				"zh": `特定服务条款`,
				"en": `Service-Specific Terms`,
				"vi": `Điều khoản riêng theo dịch vụ`,
				"ko": `서비스별 약관`,
			},
			ContentMDI18n: map[string]string{
				"zh": `服务特定条款

不同服务、模型、账号类型、套餐或 Token 产品可能适用额外限制、有效期、配额、退款规则、保修范围和上游平台条款。用户在购买、导入账号、生成 API Key 或使用具体模型前，应阅读对应页面、订单说明或管理员公告中的特定规则。若服务特定条款与通用条款不一致，以更严格或更具体的条款为准。`,
				"en": `Service-Specific Terms

Different services, models, account types, packages, or Token products may have additional limits, validity periods, quotas, refund rules, warranty scopes, and upstream platform terms. Before purchasing, importing accounts, generating API keys, or using a specific model, users should review the rules shown on the relevant page, order instructions, or administrator notices. If service-specific terms conflict with general terms, the stricter or more specific terms apply.`,
				"vi": `Điều khoản riêng theo dịch vụ

Các dịch vụ, mô hình, loại tài khoản, gói hoặc sản phẩm Token khác nhau có thể áp dụng thêm giới hạn, thời hạn hiệu lực, hạn mức, quy định hoàn tiền, phạm vi bảo hành và điều khoản của nền tảng thượng nguồn. Trước khi mua, nhập tài khoản, tạo API Key hoặc sử dụng một mô hình cụ thể, người dùng cần đọc quy định trên trang liên quan, hướng dẫn đơn hàng hoặc thông báo của quản trị viên. Nếu điều khoản riêng theo dịch vụ mâu thuẫn với điều khoản chung, điều khoản nghiêm ngặt hơn hoặc cụ thể hơn sẽ được áp dụng.`,
				"ko": `서비스별 약관

서비스, 모델, 계정 유형, 패키지 또는 토큰 상품별로 추가 제한, 유효 기간, 할당량, 환불 규정, 보증 범위 및 상위 플랫폼 약관이 적용될 수 있습니다. 구매, 계정 가져오기, API Key 생성 또는 특정 모델 사용 전에 관련 페이지, 주문 안내 또는 관리자 공지의 세부 규칙을 확인해야 합니다. 서비스별 약관이 일반 약관과 충돌하는 경우 더 엄격하거나 더 구체적인 약관이 우선합니다.`,
			},
		},
	}
}

func normalizeLoginAgreementDocumentID(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	lastSeparator := false
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			_, _ = b.WriteRune(r)
			lastSeparator = false
			continue
		}
		if r == '-' || r == '_' || r == ' ' || r == '.' || r == '/' {
			if !lastSeparator && b.Len() > 0 {
				if r == '_' {
					_, _ = b.WriteRune('_')
				} else {
					_, _ = b.WriteRune('-')
				}
				lastSeparator = true
			}
		}
	}
	return strings.Trim(b.String(), "-_")
}

func normalizeLoginAgreementLocale(raw string) string {
	locale := strings.ToLower(strings.TrimSpace(raw))
	if idx := strings.Index(locale, "-"); idx >= 0 {
		locale = locale[:idx]
	}
	switch locale {
	case "zh", "en", "vi", "ko":
		return locale
	default:
		return ""
	}
}

func normalizeLoginAgreementLocalizedMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(values))
	for key, value := range values {
		locale := normalizeLoginAgreementLocale(key)
		if locale == "" {
			continue
		}
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			normalized[locale] = trimmed
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func cloneLoginAgreementLocalizedMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneLoginAgreementDocument(doc LoginAgreementDocument) LoginAgreementDocument {
	doc.TitleI18n = cloneLoginAgreementLocalizedMap(doc.TitleI18n)
	doc.ContentMDI18n = cloneLoginAgreementLocalizedMap(doc.ContentMDI18n)
	return doc
}

// DefaultLoginAgreementDocuments returns the built-in legal documents used when
// an installation has not stored customized agreement documents yet.
func DefaultLoginAgreementDocuments() []LoginAgreementDocument {
	defaults := defaultLoginAgreementDocuments()
	cloned := make([]LoginAgreementDocument, 0, len(defaults))
	for _, doc := range defaults {
		cloned = append(cloned, cloneLoginAgreementDocument(doc))
	}
	return cloned
}

func defaultLoginAgreementDocumentByID(id string) (LoginAgreementDocument, bool) {
	for _, doc := range defaultLoginAgreementDocuments() {
		if normalizeLoginAgreementDocumentID(doc.ID) == id {
			return cloneLoginAgreementDocument(doc), true
		}
	}
	return LoginAgreementDocument{}, false
}

func mergeLoginAgreementLocalizedDefaults(current, defaults map[string]string) map[string]string {
	merged := cloneLoginAgreementLocalizedMap(current)
	if len(defaults) == 0 {
		return merged
	}
	if merged == nil {
		merged = make(map[string]string, len(defaults))
	}
	for _, locale := range []string{"zh", "en", "vi", "ko"} {
		if strings.TrimSpace(merged[locale]) == "" {
			if value := strings.TrimSpace(defaults[locale]); value != "" {
				merged[locale] = value
			}
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func firstLoginAgreementLocalizedValue(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	for _, locale := range []string{"zh", "en", "vi", "ko"} {
		if value := strings.TrimSpace(values[locale]); value != "" {
			return value
		}
	}
	return ""
}

func normalizeLoginAgreementDocuments(docs []LoginAgreementDocument) []LoginAgreementDocument {
	normalized := make([]LoginAgreementDocument, 0, len(docs))
	seen := make(map[string]int, len(docs))
	for i, doc := range docs {
		titleI18n := normalizeLoginAgreementLocalizedMap(doc.TitleI18n)
		contentI18n := normalizeLoginAgreementLocalizedMap(doc.ContentMDI18n)
		title := strings.TrimSpace(doc.Title)
		content := strings.TrimSpace(doc.ContentMD)
		if title == "" {
			title = firstLoginAgreementLocalizedValue(titleI18n)
		}
		if content == "" {
			content = firstLoginAgreementLocalizedValue(contentI18n)
		}
		id := normalizeLoginAgreementDocumentID(doc.ID)
		if id == "" {
			if title == "" && content == "" {
				continue
			}
			sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s:%s", i, title, content)))
			id = hex.EncodeToString(sum[:])[:12]
		}
		if defaults, ok := defaultLoginAgreementDocumentByID(id); ok {
			if title == "" {
				title = defaults.Title
			}
			if content == "" {
				content = defaults.ContentMD
			}
			titleI18n = mergeLoginAgreementLocalizedDefaults(titleI18n, defaults.TitleI18n)
			contentI18n = mergeLoginAgreementLocalizedDefaults(contentI18n, defaults.ContentMDI18n)
		}
		if title == "" && content == "" {
			continue
		}
		baseID := id
		for suffix := 2; seen[id] > 0; suffix++ {
			id = fmt.Sprintf("%s-%d", baseID, suffix)
		}
		seen[id]++
		normalized = append(normalized, LoginAgreementDocument{
			ID:            id,
			Title:         title,
			ContentMD:     content,
			TitleI18n:     titleI18n,
			ContentMDI18n: contentI18n,
		})
	}
	return normalized
}

func parseLoginAgreementDocuments(raw string) []LoginAgreementDocument {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultLoginAgreementDocuments()
	}
	var docs []LoginAgreementDocument
	if err := json.Unmarshal([]byte(raw), &docs); err != nil {
		return defaultLoginAgreementDocuments()
	}
	docs = normalizeLoginAgreementDocuments(docs)
	if len(docs) == 0 {
		return defaultLoginAgreementDocuments()
	}
	return docs
}

func marshalLoginAgreementDocuments(docs []LoginAgreementDocument) (string, error) {
	normalized := normalizeLoginAgreementDocuments(docs)
	if len(normalized) == 0 {
		normalized = defaultLoginAgreementDocuments()
	}
	b, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal login agreement documents: %w", err)
	}
	return string(b), nil
}

func buildLoginAgreementRevision(updatedAt string, docs []LoginAgreementDocument) string {
	normalized := normalizeLoginAgreementDocuments(docs)
	payload, err := json.Marshal(struct {
		UpdatedAt string                   `json:"updated_at"`
		Documents []LoginAgreementDocument `json:"documents"`
	}{
		UpdatedAt: strings.TrimSpace(updatedAt),
		Documents: normalized,
	})
	if err != nil {
		payload = []byte(strings.TrimSpace(updatedAt))
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])[:16]
}

// GetFrontendURL 获取前端基础URL（数据库优先，fallback 到配置文件）
func (s *SettingService) GetFrontendURL(ctx context.Context) string {
	val, err := s.settingRepo.GetValue(ctx, SettingKeyFrontendURL)
	if err == nil && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return s.cfg.Server.FrontendURL
}

// GetPublicSettings 获取公开设置（无需登录）
func (s *SettingService) GetPublicSettings(ctx context.Context) (*PublicSettings, error) {
	keys := []string{
		SettingKeyRegistrationEnabled,
		SettingKeyEmailVerifyEnabled,
		SettingKeyForceEmailOnThirdPartySignup,
		SettingKeyRegistrationEmailSuffixWhitelist,
		SettingKeyPromoCodeEnabled,
		SettingKeyPasswordResetEnabled,
		SettingKeyInvitationCodeEnabled,
		SettingKeyTotpEnabled,
		SettingKeyPasskeyEnabled,
		SettingKeyLoginAgreementEnabled,
		SettingKeyLoginAgreementMode,
		SettingKeyLoginAgreementUpdatedAt,
		SettingKeyLoginAgreementDocuments,
		SettingKeyTurnstileEnabled,
		SettingKeyTurnstileSiteKey,
		SettingKeyAPIKeyACLTrustForwardedIP,
		SettingKeySiteName,
		SettingKeySiteLogo,
		SettingKeySiteSubtitle,
		SettingKeyAPIBaseURL,
		SettingKeyContactInfo,
		SettingKeyDocURL,
		SettingKeyHomeContent,
		SettingKeyCompactHomeEnabled,
		SettingKeyHideCcsImportButton,
		SettingKeyPurchaseSubscriptionEnabled,
		SettingKeyPurchaseSubscriptionURL,
		SettingKeyTableDefaultPageSize,
		SettingKeyTablePageSizeOptions,
		SettingKeyCustomMenuItems,
		SettingKeyCustomEndpoints,
		SettingKeyLinuxDoConnectEnabled,
		SettingKeyDingTalkConnectEnabled,
		SettingKeyWeChatConnectEnabled,
		SettingKeyWeChatConnectAppID,
		SettingKeyWeChatConnectAppSecret,
		SettingKeyWeChatConnectOpenAppID,
		SettingKeyWeChatConnectOpenAppSecret,
		SettingKeyWeChatConnectMPAppID,
		SettingKeyWeChatConnectMPAppSecret,
		SettingKeyWeChatConnectMobileAppID,
		SettingKeyWeChatConnectMobileAppSecret,
		SettingKeyWeChatConnectOpenEnabled,
		SettingKeyWeChatConnectMPEnabled,
		SettingKeyWeChatConnectMobileEnabled,
		SettingKeyWeChatConnectMode,
		SettingKeyWeChatConnectScopes,
		SettingKeyWeChatConnectRedirectURL,
		SettingKeyWeChatConnectFrontendRedirectURL,
		SettingKeyBackendModeEnabled,
		SettingPaymentEnabled,
		SettingKeyOIDCConnectEnabled,
		SettingKeyOIDCConnectProviderName,
		SettingKeyGitHubOAuthEnabled,
		SettingKeyGitHubOAuthClientID,
		SettingKeyGitHubOAuthClientSecret,
		SettingKeyGoogleOAuthEnabled,
		SettingKeyGoogleOAuthClientID,
		SettingKeyGoogleOAuthClientSecret,
		SettingKeyBalanceLowNotifyEnabled,
		SettingKeyBalanceLowNotifyThreshold,
		SettingKeyBalanceLowNotifyRechargeURL,
		SettingKeyAccountQuotaNotifyEnabled,
		SettingKeyChannelMonitorEnabled,
		SettingKeyChannelMonitorDefaultIntervalSeconds,
		SettingKeyAvailableChannelsEnabled,
		SettingKeyModelPlazaEnabled,
		SettingKeyModelPlazaRequireAuth,
		SettingKeyAffiliateEnabled,
		SettingKeyDeviceAutoActivationAffCodes,
		SettingKeyRiskControlEnabled,
		SettingKeyAllowUserViewErrorRequests,
	}

	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("get public settings: %w", err)
	}

	linuxDoEnabled := false
	if raw, ok := settings[SettingKeyLinuxDoConnectEnabled]; ok {
		linuxDoEnabled = raw == "true"
	} else {
		linuxDoEnabled = s.cfg != nil && s.cfg.LinuxDo.Enabled
	}
	dingTalkEnabled := false
	if raw, ok := settings[SettingKeyDingTalkConnectEnabled]; ok {
		dingTalkEnabled = raw == "true"
	} else {
		dingTalkEnabled = s.cfg != nil && s.cfg.DingTalk.Enabled
	}
	oidcEnabled := false
	if raw, ok := settings[SettingKeyOIDCConnectEnabled]; ok {
		oidcEnabled = raw == "true"
	} else {
		oidcEnabled = s.cfg != nil && s.cfg.OIDC.Enabled
	}
	oidcProviderName := strings.TrimSpace(settings[SettingKeyOIDCConnectProviderName])
	if oidcProviderName == "" && s.cfg != nil {
		oidcProviderName = strings.TrimSpace(s.cfg.OIDC.ProviderName)
	}
	if oidcProviderName == "" {
		oidcProviderName = "OIDC"
	}
	gitHubEnabled := s.emailOAuthPublicEnabled(settings, "github")
	googleEnabled := s.emailOAuthPublicEnabled(settings, "google")
	weChatEnabled, weChatOpenEnabled, weChatMPEnabled, weChatMobileEnabled := s.weChatOAuthCapabilitiesFromSettings(settings)

	// Password reset requires email verification to be enabled
	emailVerifyEnabled := settings[SettingKeyEmailVerifyEnabled] == "true"
	passwordResetEnabled := emailVerifyEnabled && settings[SettingKeyPasswordResetEnabled] == "true"
	registrationEmailSuffixWhitelist := ParseRegistrationEmailSuffixWhitelist(
		settings[SettingKeyRegistrationEmailSuffixWhitelist],
	)
	tableDefaultPageSize, tablePageSizeOptions := parseTablePreferences(
		settings[SettingKeyTableDefaultPageSize],
		settings[SettingKeyTablePageSizeOptions],
	)
	loginAgreementDocuments := parseLoginAgreementDocuments(settings[SettingKeyLoginAgreementDocuments])
	loginAgreementUpdatedAt := strings.TrimSpace(settings[SettingKeyLoginAgreementUpdatedAt])
	if loginAgreementUpdatedAt == "" {
		loginAgreementUpdatedAt = defaultLoginAgreementDate
	}

	var balanceLowNotifyThreshold float64
	if v, err := strconv.ParseFloat(settings[SettingKeyBalanceLowNotifyThreshold], 64); err == nil && v >= 0 {
		balanceLowNotifyThreshold = v
	}

	return &PublicSettings{
		RegistrationEnabled:              settings[SettingKeyRegistrationEnabled] == "true",
		EmailVerifyEnabled:               emailVerifyEnabled,
		ForceEmailOnThirdPartySignup:     settings[SettingKeyForceEmailOnThirdPartySignup] == "true",
		RegistrationEmailSuffixWhitelist: registrationEmailSuffixWhitelist,
		PromoCodeEnabled:                 settings[SettingKeyPromoCodeEnabled] != "false", // 默认启用
		PasswordResetEnabled:             passwordResetEnabled,
		InvitationCodeEnabled:            settings[SettingKeyInvitationCodeEnabled] == "true",
		TotpEnabled:                      settings[SettingKeyTotpEnabled] == "true",
		PasskeyEnabled:                   s.passkeyConfigured() && s.passkeySettingEnabled(settings),
		LoginAgreementEnabled:            settings[SettingKeyLoginAgreementEnabled] == "true" && len(loginAgreementDocuments) > 0,
		LoginAgreementMode:               normalizeLoginAgreementMode(settings[SettingKeyLoginAgreementMode]),
		LoginAgreementUpdatedAt:          loginAgreementUpdatedAt,
		LoginAgreementRevision:           buildLoginAgreementRevision(loginAgreementUpdatedAt, loginAgreementDocuments),
		LoginAgreementDocuments:          loginAgreementDocuments,
		TurnstileEnabled:                 settings[SettingKeyTurnstileEnabled] == "true",
		TurnstileSiteKey:                 settings[SettingKeyTurnstileSiteKey],
		SiteName:                         s.getStringOrDefault(settings, SettingKeySiteName, "Sub2API"),
		SiteLogo:                         settings[SettingKeySiteLogo],
		SiteSubtitle:                     s.getStringOrDefault(settings, SettingKeySiteSubtitle, "Subscription to API Conversion Platform"),
		APIBaseURL:                       settings[SettingKeyAPIBaseURL],
		ContactInfo:                      settings[SettingKeyContactInfo],
		DocURL:                           settings[SettingKeyDocURL],
		HomeContent:                      settings[SettingKeyHomeContent],
		CompactHomeEnabled:               settings[SettingKeyCompactHomeEnabled] == "true",
		HideCcsImportButton:              settings[SettingKeyHideCcsImportButton] == "true",
		PurchaseSubscriptionEnabled:      settings[SettingKeyPurchaseSubscriptionEnabled] == "true",
		PurchaseSubscriptionURL:          strings.TrimSpace(settings[SettingKeyPurchaseSubscriptionURL]),
		TableDefaultPageSize:             tableDefaultPageSize,
		TablePageSizeOptions:             tablePageSizeOptions,
		CustomMenuItems:                  settings[SettingKeyCustomMenuItems],
		CustomEndpoints:                  settings[SettingKeyCustomEndpoints],
		LinuxDoOAuthEnabled:              linuxDoEnabled,
		DingTalkOAuthEnabled:             dingTalkEnabled,
		WeChatOAuthEnabled:               weChatEnabled,
		WeChatOAuthOpenEnabled:           weChatOpenEnabled,
		WeChatOAuthMPEnabled:             weChatMPEnabled,
		WeChatOAuthMobileEnabled:         weChatMobileEnabled,
		BackendModeEnabled:               settings[SettingKeyBackendModeEnabled] == "true",
		PaymentEnabled:                   settings[SettingPaymentEnabled] == "true",
		OIDCOAuthEnabled:                 oidcEnabled,
		OIDCOAuthProviderName:            oidcProviderName,
		GitHubOAuthEnabled:               gitHubEnabled,
		GoogleOAuthEnabled:               googleEnabled,
		BalanceLowNotifyEnabled:          settings[SettingKeyBalanceLowNotifyEnabled] == "true",
		AccountQuotaNotifyEnabled:        settings[SettingKeyAccountQuotaNotifyEnabled] == "true",
		BalanceLowNotifyThreshold:        balanceLowNotifyThreshold,
		BalanceLowNotifyRechargeURL:      settings[SettingKeyBalanceLowNotifyRechargeURL],

		ChannelMonitorEnabled:                !isFalseSettingValue(settings[SettingKeyChannelMonitorEnabled]),
		ChannelMonitorDefaultIntervalSeconds: parseChannelMonitorInterval(settings[SettingKeyChannelMonitorDefaultIntervalSeconds]),

		AvailableChannelsEnabled: settings[SettingKeyAvailableChannelsEnabled] == "true",

		ModelPlazaEnabled:     settings[SettingKeyModelPlazaEnabled] == "true",
		ModelPlazaRequireAuth: settings[SettingKeyModelPlazaRequireAuth] == "true",

		AffiliateEnabled:             settings[SettingKeyAffiliateEnabled] == "true",
		DeviceAutoActivationAffCodes: deviceAutoActivationAffCodesSetting(settings),

		RiskControlEnabled: settings[SettingKeyRiskControlEnabled] == "true",

		AllowUserViewErrorRequests: settings[SettingKeyAllowUserViewErrorRequests] == "true",
	}, nil
}

// channelMonitorIntervalMin / channelMonitorIntervalMax bound the default interval
// (mirrors the monitor-level constraint but lives here so setting_service stays decoupled).
const (
	channelMonitorIntervalMin      = 15
	channelMonitorIntervalMax      = 3600
	channelMonitorIntervalFallback = 60
)

// parseChannelMonitorInterval parses the stored string and clamps to [15, 3600].
// Empty / invalid input falls back to channelMonitorIntervalFallback.
func parseChannelMonitorInterval(raw string) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return channelMonitorIntervalFallback
	}
	return clampChannelMonitorInterval(v)
}

// clampChannelMonitorInterval clamps v to the allowed range. 0 means "not provided".
func clampChannelMonitorInterval(v int) int {
	if v <= 0 {
		return 0
	}
	if v < channelMonitorIntervalMin {
		return channelMonitorIntervalMin
	}
	if v > channelMonitorIntervalMax {
		return channelMonitorIntervalMax
	}
	return v
}

// ChannelMonitorRuntime is the lightweight view of the channel monitor feature
// consumed by the runner and user-facing handlers.
type ChannelMonitorRuntime struct {
	Enabled                bool
	DefaultIntervalSeconds int
}

// GetChannelMonitorRuntime reads the channel monitor feature flags directly from
// the settings store. Fail-open: on error returns Enabled=true with the default interval.
func (s *SettingService) GetChannelMonitorRuntime(ctx context.Context) ChannelMonitorRuntime {
	vals, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyChannelMonitorEnabled,
		SettingKeyChannelMonitorDefaultIntervalSeconds,
	})
	if err != nil {
		return ChannelMonitorRuntime{Enabled: true, DefaultIntervalSeconds: channelMonitorIntervalFallback}
	}
	return ChannelMonitorRuntime{
		Enabled:                !isFalseSettingValue(vals[SettingKeyChannelMonitorEnabled]),
		DefaultIntervalSeconds: parseChannelMonitorInterval(vals[SettingKeyChannelMonitorDefaultIntervalSeconds]),
	}
}

// AvailableChannelsRuntime is the lightweight view of the available-channels feature
// switch consumed by the user-facing handler.
type AvailableChannelsRuntime struct {
	Enabled bool
}

// GetAvailableChannelsRuntime reads the available-channels feature switch directly
// from the settings store. Fail-closed: on error returns Enabled=false, matching
// the opt-in default (unknown ↔ disabled).
func (s *SettingService) GetAvailableChannelsRuntime(ctx context.Context) AvailableChannelsRuntime {
	vals, err := s.settingRepo.GetMultiple(ctx, []string{SettingKeyAvailableChannelsEnabled})
	if err != nil {
		return AvailableChannelsRuntime{Enabled: false}
	}
	return AvailableChannelsRuntime{
		Enabled: vals[SettingKeyAvailableChannelsEnabled] == "true",
	}
}

// ModelPlazaRuntime is the lightweight view of the model-plaza feature consumed
// by the public plaza handler.
type ModelPlazaRuntime struct {
	Enabled     bool
	RequireAuth bool
	Description string
}

// GetModelPlazaRuntime reads the model-plaza feature switches directly from the
// settings store. Fail-closed: on error returns Enabled=false, matching the
// opt-in default (unknown ↔ disabled).
func (s *SettingService) GetModelPlazaRuntime(ctx context.Context) ModelPlazaRuntime {
	vals, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyModelPlazaEnabled,
		SettingKeyModelPlazaRequireAuth,
		SettingKeyModelPlazaDescription,
	})
	if err != nil {
		return ModelPlazaRuntime{Enabled: false}
	}
	return ModelPlazaRuntime{
		Enabled:     vals[SettingKeyModelPlazaEnabled] == "true",
		RequireAuth: vals[SettingKeyModelPlazaRequireAuth] == "true",
		Description: vals[SettingKeyModelPlazaDescription],
	}
}

// IsUserErrorViewAllowed reads the user-facing error-requests visibility switch
// directly from the settings store. Fail-closed: on error returns false (opt-in default).
func (s *SettingService) IsUserErrorViewAllowed(ctx context.Context) bool {
	vals, err := s.settingRepo.GetMultiple(ctx, []string{SettingKeyAllowUserViewErrorRequests})
	if err != nil {
		slog.Warn("failed to get allow_user_view_error_requests setting, defaulting to false", "error", err)
		return false
	}
	return vals[SettingKeyAllowUserViewErrorRequests] == "true"
}

// PublicSettingsInjectionPayload is the JSON shape embedded into HTML as
// `window.__APP_CONFIG__` so the frontend can hydrate feature flags & site
// config before the first XHR finishes.
//
// INVARIANT: every `json` tag here MUST also exist on handler/dto.PublicSettings.
// If you forget a feature-flag field here, the frontend's
// `cachedPublicSettings.xxx_enabled` will be `undefined` on refresh until the
// async `/api/v1/settings/public` call returns — which causes opt-in menus
// (strict `=== true`) to flicker off/on. See
// frontend/src/utils/featureFlags.ts for the matching registry.
//
// A unit test diffs this struct's JSON keys against dto.PublicSettings to catch
// drift automatically (see setting_service_injection_test.go).
type PublicSettingsInjectionPayload struct {
	RegistrationEnabled              bool                     `json:"registration_enabled"`
	EmailVerifyEnabled               bool                     `json:"email_verify_enabled"`
	RegistrationEmailSuffixWhitelist []string                 `json:"registration_email_suffix_whitelist"`
	PromoCodeEnabled                 bool                     `json:"promo_code_enabled"`
	PasswordResetEnabled             bool                     `json:"password_reset_enabled"`
	InvitationCodeEnabled            bool                     `json:"invitation_code_enabled"`
	TotpEnabled                      bool                     `json:"totp_enabled"`
	PasskeyEnabled                   bool                     `json:"passkey_enabled"`
	LoginAgreementEnabled            bool                     `json:"login_agreement_enabled"`
	LoginAgreementMode               string                   `json:"login_agreement_mode"`
	LoginAgreementUpdatedAt          string                   `json:"login_agreement_updated_at"`
	LoginAgreementRevision           string                   `json:"login_agreement_revision"`
	LoginAgreementDocuments          []LoginAgreementDocument `json:"login_agreement_documents"`
	TurnstileEnabled                 bool                     `json:"turnstile_enabled"`
	TurnstileSiteKey                 string                   `json:"turnstile_site_key"`
	SiteName                         string                   `json:"site_name"`
	SiteLogo                         string                   `json:"site_logo"`
	SiteSubtitle                     string                   `json:"site_subtitle"`
	APIBaseURL                       string                   `json:"api_base_url"`
	ContactInfo                      string                   `json:"contact_info"`
	DocURL                           string                   `json:"doc_url"`
	HomeContent                      string                   `json:"home_content"`
	CompactHomeEnabled               bool                     `json:"compact_home_enabled"`
	HideCcsImportButton              bool                     `json:"hide_ccs_import_button"`
	PurchaseSubscriptionEnabled      bool                     `json:"purchase_subscription_enabled"`
	PurchaseSubscriptionURL          string                   `json:"purchase_subscription_url"`
	TableDefaultPageSize             int                      `json:"table_default_page_size"`
	TablePageSizeOptions             []int                    `json:"table_page_size_options"`
	CustomMenuItems                  json.RawMessage          `json:"custom_menu_items"`
	CustomEndpoints                  json.RawMessage          `json:"custom_endpoints"`
	LinuxDoOAuthEnabled              bool                     `json:"linuxdo_oauth_enabled"`
	DingTalkOAuthEnabled             bool                     `json:"dingtalk_oauth_enabled"`
	WeChatOAuthEnabled               bool                     `json:"wechat_oauth_enabled"`
	WeChatOAuthOpenEnabled           bool                     `json:"wechat_oauth_open_enabled"`
	WeChatOAuthMPEnabled             bool                     `json:"wechat_oauth_mp_enabled"`
	WeChatOAuthMobileEnabled         bool                     `json:"wechat_oauth_mobile_enabled"`
	OIDCOAuthEnabled                 bool                     `json:"oidc_oauth_enabled"`
	OIDCOAuthProviderName            string                   `json:"oidc_oauth_provider_name"`
	GitHubOAuthEnabled               bool                     `json:"github_oauth_enabled"`
	GoogleOAuthEnabled               bool                     `json:"google_oauth_enabled"`
	BackendModeEnabled               bool                     `json:"backend_mode_enabled"`
	PaymentEnabled                   bool                     `json:"payment_enabled"`
	Version                          string                   `json:"version"`
	// 服务器全局时区（IANA 名称与当前 UTC 偏移），高峰时段等服务端本地时间窗口的展示标注用
	ServerTimezone              string  `json:"server_timezone"`
	ServerUTCOffset             string  `json:"server_utc_offset"`
	BalanceLowNotifyEnabled     bool    `json:"balance_low_notify_enabled"`
	AccountQuotaNotifyEnabled   bool    `json:"account_quota_notify_enabled"`
	BalanceLowNotifyThreshold   float64 `json:"balance_low_notify_threshold"`
	BalanceLowNotifyRechargeURL string  `json:"balance_low_notify_recharge_url"`

	// Feature flags — MUST match the opt-in/opt-out registry in
	// frontend/src/utils/featureFlags.ts. Missing a field here is the bug
	// that hid the "可用渠道" menu on page refresh.
	ChannelMonitorEnabled                bool   `json:"channel_monitor_enabled"`
	ChannelMonitorDefaultIntervalSeconds int    `json:"channel_monitor_default_interval_seconds"`
	AvailableChannelsEnabled             bool   `json:"available_channels_enabled"`
	ModelPlazaEnabled                    bool   `json:"model_plaza_enabled"`
	ModelPlazaRequireAuth                bool   `json:"model_plaza_require_auth"`
	AffiliateEnabled                     bool   `json:"affiliate_enabled"`
	DeviceAutoActivationAffCodes         string `json:"device_auto_activation_aff_codes"`
	RiskControlEnabled                   bool   `json:"risk_control_enabled"`
	AllowUserViewErrorRequests           bool   `json:"allow_user_view_error_requests"`
}

// GetPublicSettingsForInjection returns public settings in a format suitable for HTML injection.
// This implements the web.PublicSettingsProvider interface.
func (s *SettingService) GetPublicSettingsForInjection(ctx context.Context) (any, error) {
	settings, err := s.GetPublicSettings(ctx)
	if err != nil {
		return nil, err
	}

	return &PublicSettingsInjectionPayload{
		RegistrationEnabled:              settings.RegistrationEnabled,
		EmailVerifyEnabled:               settings.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist: settings.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                 settings.PromoCodeEnabled,
		PasswordResetEnabled:             settings.PasswordResetEnabled,
		InvitationCodeEnabled:            settings.InvitationCodeEnabled,
		TotpEnabled:                      settings.TotpEnabled,
		PasskeyEnabled:                   settings.PasskeyEnabled,
		LoginAgreementEnabled:            settings.LoginAgreementEnabled,
		LoginAgreementMode:               settings.LoginAgreementMode,
		LoginAgreementUpdatedAt:          settings.LoginAgreementUpdatedAt,
		LoginAgreementRevision:           settings.LoginAgreementRevision,
		LoginAgreementDocuments:          settings.LoginAgreementDocuments,
		TurnstileEnabled:                 settings.TurnstileEnabled,
		TurnstileSiteKey:                 settings.TurnstileSiteKey,
		SiteName:                         settings.SiteName,
		SiteLogo:                         settings.SiteLogo,
		SiteSubtitle:                     settings.SiteSubtitle,
		APIBaseURL:                       settings.APIBaseURL,
		ContactInfo:                      settings.ContactInfo,
		DocURL:                           settings.DocURL,
		HomeContent:                      settings.HomeContent,
		CompactHomeEnabled:               settings.CompactHomeEnabled,
		HideCcsImportButton:              settings.HideCcsImportButton,
		PurchaseSubscriptionEnabled:      settings.PurchaseSubscriptionEnabled,
		PurchaseSubscriptionURL:          settings.PurchaseSubscriptionURL,
		TableDefaultPageSize:             settings.TableDefaultPageSize,
		TablePageSizeOptions:             settings.TablePageSizeOptions,
		CustomMenuItems:                  filterUserVisibleMenuItems(settings.CustomMenuItems),
		CustomEndpoints:                  safeRawJSONArray(settings.CustomEndpoints),
		LinuxDoOAuthEnabled:              settings.LinuxDoOAuthEnabled,
		DingTalkOAuthEnabled:             settings.DingTalkOAuthEnabled,
		WeChatOAuthEnabled:               settings.WeChatOAuthEnabled,
		WeChatOAuthOpenEnabled:           settings.WeChatOAuthOpenEnabled,
		WeChatOAuthMPEnabled:             settings.WeChatOAuthMPEnabled,
		WeChatOAuthMobileEnabled:         settings.WeChatOAuthMobileEnabled,
		OIDCOAuthEnabled:                 settings.OIDCOAuthEnabled,
		OIDCOAuthProviderName:            settings.OIDCOAuthProviderName,
		GitHubOAuthEnabled:               settings.GitHubOAuthEnabled,
		GoogleOAuthEnabled:               settings.GoogleOAuthEnabled,
		BackendModeEnabled:               settings.BackendModeEnabled,
		PaymentEnabled:                   settings.PaymentEnabled,
		Version:                          s.version,
		ServerTimezone:                   timezone.Name(),
		ServerUTCOffset:                  timezone.UTCOffset(),
		BalanceLowNotifyEnabled:          settings.BalanceLowNotifyEnabled,
		AccountQuotaNotifyEnabled:        settings.AccountQuotaNotifyEnabled,
		BalanceLowNotifyThreshold:        settings.BalanceLowNotifyThreshold,
		BalanceLowNotifyRechargeURL:      settings.BalanceLowNotifyRechargeURL,

		ChannelMonitorEnabled:                settings.ChannelMonitorEnabled,
		ChannelMonitorDefaultIntervalSeconds: settings.ChannelMonitorDefaultIntervalSeconds,
		AvailableChannelsEnabled:             settings.AvailableChannelsEnabled,
		ModelPlazaEnabled:                    settings.ModelPlazaEnabled,
		ModelPlazaRequireAuth:                settings.ModelPlazaRequireAuth,
		AffiliateEnabled:                     settings.AffiliateEnabled,
		DeviceAutoActivationAffCodes:         settings.DeviceAutoActivationAffCodes,
		RiskControlEnabled:                   settings.RiskControlEnabled,
		AllowUserViewErrorRequests:           settings.AllowUserViewErrorRequests,
	}, nil
}

// filterUserVisibleMenuItems filters out admin-only menu items from a raw JSON
// array string, returning only items with visibility != "admin".
func filterUserVisibleMenuItems(raw string) json.RawMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return json.RawMessage("[]")
	}
	var items []struct {
		Visibility string `json:"visibility"`
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return json.RawMessage("[]")
	}

	// Parse full items to preserve all fields
	var fullItems []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fullItems); err != nil {
		return json.RawMessage("[]")
	}

	var filtered []json.RawMessage
	for i, item := range items {
		if item.Visibility != "admin" {
			filtered = append(filtered, fullItems[i])
		}
	}
	if len(filtered) == 0 {
		return json.RawMessage("[]")
	}
	result, err := json.Marshal(filtered)
	if err != nil {
		return json.RawMessage("[]")
	}
	return result
}

// safeRawJSONArray returns raw as json.RawMessage if it's valid JSON, otherwise "[]".
func safeRawJSONArray(raw string) json.RawMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return json.RawMessage("[]")
	}
	if json.Valid([]byte(raw)) {
		return json.RawMessage(raw)
	}
	return json.RawMessage("[]")
}

// GetFrameSrcOrigins returns deduplicated http(s) origins from home_content URL,
// purchase_subscription_url, and all custom_menu_items URLs. Used by the router layer for CSP frame-src injection.
func (s *SettingService) GetFrameSrcOrigins(ctx context.Context) ([]string, error) {
	settings, err := s.GetPublicSettings(ctx)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var origins []string

	addOrigin := func(rawURL string) {
		if origin := extractOriginFromURL(rawURL); origin != "" {
			if _, ok := seen[origin]; !ok {
				seen[origin] = struct{}{}
				origins = append(origins, origin)
			}
		}
	}

	// home content URL (when home_content is set to a URL for iframe embedding)
	addOrigin(settings.HomeContent)

	// purchase subscription URL
	if settings.PurchaseSubscriptionEnabled {
		addOrigin(settings.PurchaseSubscriptionURL)
	}

	// all custom menu items (including admin-only, since CSP must allow all iframes)
	for _, item := range parseCustomMenuItemURLs(settings.CustomMenuItems) {
		addOrigin(item)
	}

	return origins, nil
}

// extractOriginFromURL returns the scheme+host origin from rawURL.
// Only http and https schemes are accepted.
func extractOriginFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// parseCustomMenuItemURLs extracts URLs from a raw JSON array of custom menu items.
func parseCustomMenuItemURLs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	var items []struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	urls := make([]string, 0, len(items))
	for _, item := range items {
		if item.URL != "" {
			urls = append(urls, item.URL)
		}
	}
	return urls
}
