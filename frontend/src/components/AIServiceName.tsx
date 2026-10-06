import { AI_SERVICE_ICONS } from "../aiServices";
import { useT } from "../i18n";

export function AIServiceName({ service }: { service: string }) {
  const t = useT();
  const icon = AI_SERVICE_ICONS[service];
  return (
    <span className="ai-service-name">
      {icon && <span className="ai-service-icon" aria-hidden="true" style={{ maskImage: `url("${icon}")`, WebkitMaskImage: `url("${icon}")` }} />}
      {t(service)}
    </span>
  );
}
