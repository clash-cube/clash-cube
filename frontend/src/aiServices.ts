import openai from "./assets/ai/openai.svg";
import claude from "./assets/ai/claude.svg";

// Names accepted by backend/aicheck.go. Register each service's icon here
// so Settings and Overview share both the available services and branding.
export const AI_SERVICE_ICONS: Record<string, string> = { OpenAI: openai, Claude: claude };
export const AI_SERVICES = Object.keys(AI_SERVICE_ICONS);
