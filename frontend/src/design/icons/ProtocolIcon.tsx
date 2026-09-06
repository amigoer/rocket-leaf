import type { CSSProperties } from "react";
import {
  SiApache,
  SiApachekafka,
  SiApachepulsar,
  SiApacherocketmq,
  SiGooglepubsub,
  SiMqtt,
  SiNatsdotio,
  SiRabbitmq,
  SiRedis,
} from "react-icons/si";
import type { IconBaseProps, IconType } from "react-icons";
import type { ProtocolId } from "@/design/data/protocols";

/*
 * The canvas pulls these from cdn.simpleicons.org. A packaged desktop app has
 * no network guarantee, so the same Simple Icons glyphs are bundled through
 * react-icons and tinted with each brand's documented hex.
 *
 * Two of those hexes are near-black and would disappear on a dark ground, so
 * they go through a token that carries the brand's own dark-mode value. The
 * other five read on both.
 *
 * NSQ and SQS are drawn here instead. Simple Icons carries neither: NSQ's own
 * mark is a wordmark rather than a symbol, and Amazon's were withdrawn from the
 * set over trademark policy. So rather than borrowing a neighbour's, each has a
 * mark of its own, tinted through a token because a colour invented here has no
 * brand value to be faithful to.
 */
/**
 * One topic fanning out into three channels, which is the whole of what NSQ
 * does that the other families do not: every channel under a topic receives a
 * copy of every message, so the fan-out is the family rather than a detail of
 * it.
 */
function NsqGlyph({ size = 14, color = "currentColor", ...rest }: IconBaseProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke={color}
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...rest}
    >
      <circle cx="4" cy="12" r="2.5" fill={color} stroke="none" />
      <circle cx="20" cy="4" r="2" />
      <circle cx="20" cy="12" r="2" />
      <circle cx="20" cy="20" r="2" />
      <path d="M6.5 12h4M10.5 12 18 4.6M10.5 12H18M10.5 12 18 19.4" />
    </svg>
  );
}

/**
 * Messages in line, and one leaving. It is the whole of what SQS is: a queue
 * holds what has been sent until somebody asks for it, with no topic, no
 * subscription and no second object anywhere in the service.
 */
function SqsGlyph({ size = 14, color = "currentColor", ...rest }: IconBaseProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="none"
      stroke={color}
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...rest}
    >
      <rect x="2" y="8" width="4" height="8" rx="1" />
      <rect x="8" y="8" width="4" height="8" rx="1" />
      <rect x="14" y="8" width="4" height="8" rx="1" fill={color} stroke="none" />
      <path d="M20 12h2M20.5 9.5 23 12l-2.5 2.5" />
    </svg>
  );
}

const GLYPH: Record<ProtocolId, { icon: IconType; color: string }> = {
  rocketmq: { icon: SiApacherocketmq, color: "#D77310" },
  kafka: { icon: SiApachekafka, color: "var(--c-brand-kafka)" },
  rabbitmq: { icon: SiRabbitmq, color: "#FF6600" },
  pulsar: { icon: SiApachepulsar, color: "#188FFF" },
  redis: { icon: SiRedis, color: "#FF4438" },
  mqtt: { icon: SiMqtt, color: "var(--c-brand-mqtt)" },
  nats: { icon: SiNatsdotio, color: "#27AAE1" },
  // Simple Icons has no ActiveMQ glyph, for either product, so this is
  // the Apache feather in the foundation's own red. Better a mark that
  // is true than a neighbour's borrowed.
  activemq: { icon: SiApache, color: "#D22128" },
  nsq: { icon: NsqGlyph, color: "var(--c-brand-nsq)" },
  sqs: { icon: SqsGlyph, color: "var(--c-brand-sqs)" },
  // Simple Icons does carry this one, in a pale blue meant for a dark
  // ground, so the token is what keeps it legible on a light one.
  "google-pubsub": { icon: SiGooglepubsub, color: "var(--c-brand-google-pubsub)" },
};

export function ProtocolIcon({
  protocol,
  size = 14,
  className = "plogo",
  style,
}: {
  protocol: ProtocolId;
  size?: number;
  className?: string;
  style?: CSSProperties;
}) {
  const { icon: Icon, color } = GLYPH[protocol];
  return (
    <Icon
      aria-hidden
      className={className}
      size={size}
      color={color}
      style={style}
    />
  );
}
