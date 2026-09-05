import type { CSSProperties } from "react";
import {
  SiApache,
  SiApachekafka,
  SiApachepulsar,
  SiApacherocketmq,
  SiMqtt,
  SiNatsdotio,
  SiRabbitmq,
  SiRedis,
} from "react-icons/si";
import type { IconType } from "react-icons";
import type { ProtocolId } from "@/design/data/protocols";

/*
 * The canvas pulls these from cdn.simpleicons.org. A packaged desktop app has
 * no network guarantee, so the same Simple Icons glyphs are bundled through
 * react-icons and tinted with each brand's documented hex.
 *
 * Two of those hexes are near-black and would disappear on a dark ground, so
 * they go through a token that carries the brand's own dark-mode value. The
 * other four read on both.
 */
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
