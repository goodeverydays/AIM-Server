package llm

import (
	"fmt"
	"strings"
)

// ============================================================
// 技能分发（方式B核心）
// ------------------------------------------------------------
// 所有 AI 能力通过 ProcessMessage 的 skill 字段路由，不新增 gRPC 接口。
// 每个技能 = 一段系统提示词 + 一种上下文拼装方式。
// 新增技能：在 skillRegistry 里加一条即可，无需改 proto/IMServer/客户端结构。
// ============================================================

// 技能标识常量。客户端/IMServer 透传这些字符串。
const (
	SkillChat        = "chat"          // 普通对话（默认）
	SkillSummarize   = "summarize"     // 聊天总结
	SkillSuggestReply = "suggest_reply" // 智能回复建议
	SkillSocialAction = "social_action" // 社交意图：加好友/加群（输出结构化JSON）
	SkillRagQa       = "rag_qa"        // 基于聊天记录的检索增强问答（RAG）
)

// skillSpec 描述一个技能的提示词与行为。
type skillSpec struct {
	// system 是该技能的系统提示词，定义 LLM 的角色与输出要求。
	system string
	// needContext 表示该技能是否依赖 context（历史消息）。
	needContext bool
}

// skillRegistry 是技能注册表。加新技能只需在此追加一条。
var skillRegistry = map[string]skillSpec{
	SkillChat: {
		system:      "你是一个友好的即时通讯智能助手，用简洁自然的中文回答用户问题。",
		needContext: false,
	},
	SkillSummarize: {
		system: "你是一个聊天记录总结助手。请阅读下面提供的聊天记录，" +
			"用简体中文输出一段简明摘要，提炼出主要话题、关键结论和待办事项。" +
			"如果聊天记录为空或无实质内容，请直接说明。",
		needContext: true,
	},
	SkillSuggestReply: {
		system: "你是一个回复建议助手。请根据下面的聊天上下文，" +
			"站在“我”的角度，给出 3 条简短、得体、风格各异的候选回复，" +
			"每条单独成行，用“1. ”“2. ”“3. ”编号，不要额外解释。",
		needContext: true,
	},
	SkillRagQa: {
		system: "你是基于用户聊天记录的问答助手。下面【聊天记录】里是通过向量检索从该用户的历史消息中找出的、" +
			"与问题最相关的若干片段。请【仅依据这些片段】用简体中文回答用户的问题：\n" +
			"- 若片段中含答案，简明作答，并尽量点明是谁、大致什么时候说的；\n" +
			"- 若片段不足以回答，直接说“在你的聊天记录里没找到相关信息”，不要编造、不要臆测。",
		needContext: true,
	},
	SkillSocialAction: {
		system: "你是 IM 客户端的社交助手，既能闲聊，也能帮用户【加好友】或【申请加入群聊】。\n" +
			"请分析用户这句话的意图，并【只】输出一个严格的 JSON 对象（不要包含任何额外文字、解释或 markdown 代码块），字段如下：\n" +
			"{\n" +
			"  \"action\": \"add_friend\" 或 \"join_group\" 或 \"chat\",\n" +
			"  \"target_id\": 要加为好友的用户数字ID（仅 add_friend 时填，否则填 0）,\n" +
			"  \"group_id\": 要加入的群数字ID（仅 join_group 时填，否则填 0）,\n" +
			"  \"reply\": 用简体中文对用户说的话\n" +
			"}\n" +
			"判定规则：\n" +
			"- 用户想加某人为好友且给出了对方的数字ID → action=\"add_friend\"，target_id=该ID，reply 自然语言确认。\n" +
			"- 用户想加入/申请某个群且给出了群的数字ID → action=\"join_group\"，group_id=该ID，reply 确认。\n" +
			"- 用户想加好友或加群但没有提供数字ID → action=\"chat\"，reply 友好地询问对方的用户ID或群号。\n" +
			"- 其它任何情况（闲聊、提问等）→ action=\"chat\"，reply 为正常回答。\n" +
			"再次强调：只输出 JSON，不要输出任何其它内容。",
		needContext: false,
	},
}

// NormalizeSkill 将空值归一为默认对话技能。
func NormalizeSkill(skill string) string {
	if skill == "" {
		return SkillChat
	}
	return skill
}

// IsKnownSkill 判断技能是否已注册。
func IsKnownSkill(skill string) bool {
	_, ok := skillRegistry[NormalizeSkill(skill)]
	return ok
}

// SystemPrompt 返回指定技能的系统提示词；未知技能回退到普通对话。
func SystemPrompt(skill string) string {
	spec, ok := skillRegistry[NormalizeSkill(skill)]
	if !ok {
		return skillRegistry[SkillChat].system
	}
	return spec.system
}

// BuildUserContent 只拼装"上下文 + 用户输入"部分（不含系统提示词），
// 供 Chat API 使用：系统提示词单独放进 system 消息，这里作为 user 消息内容。
func BuildUserContent(req *ChatRequest) string {
	skill := NormalizeSkill(req.Skill)
	spec, ok := skillRegistry[skill]
	if !ok {
		spec = skillRegistry[SkillChat]
	}

	var sb strings.Builder
	if spec.needContext && len(req.Context) > 0 {
		sb.WriteString("【聊天记录】\n")
		for _, line := range req.Context {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	if req.Content != "" {
		sb.WriteString(req.Content)
	}
	return sb.String()
}

// BuildPrompt 根据技能把系统提示词、上下文消息、用户输入拼成最终 prompt。
// 真实 Provider 接入后可直接用它构造发送给 LLM 的内容；
// 也可只取 SystemPrompt 走多角色消息格式（按各 Provider 习惯）。
func BuildPrompt(req *ChatRequest) string {
	skill := NormalizeSkill(req.Skill)
	spec, ok := skillRegistry[skill]
	if !ok {
		spec = skillRegistry[SkillChat]
	}

	var sb strings.Builder
	sb.WriteString(spec.system)
	sb.WriteString("\n\n")

	if spec.needContext && len(req.Context) > 0 {
		sb.WriteString("【聊天记录】\n")
		for _, line := range req.Context {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if req.Content != "" {
		sb.WriteString(fmt.Sprintf("【用户指令】\n%s\n", req.Content))
	}

	return sb.String()
}
