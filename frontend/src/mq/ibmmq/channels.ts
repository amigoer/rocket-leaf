/**
 * How a channel reads on the page.
 *
 * The type is what everything else hangs off, and the three groups do not
 * overlap. A server-connection channel is where client applications arrive and
 * has one running instance per connected application. A message channel moves
 * messages between queue managers and works in one direction, so a sender and
 * a receiver are two halves of one link on two queue managers. A cluster
 * channel is a message channel a queue manager often defines for itself from a
 * repository's information rather than one an administrator wrote.
 *
 * A client-connection channel is the odd one: it is a definition this queue
 * manager holds on behalf of client applications, and it never runs here. A
 * row with no status is correct for it and would be a fault on a sender.
 */
import { ChannelStatus, ChannelType, type Channel } from "@bindings/model/models";

/** Statuses that mean somebody should look. */
const UNHEALTHY: ReadonlySet<string> = new Set<string>([
  ChannelStatus.ChannelRetrying,
  ChannelStatus.ChannelStopped,
  ChannelStatus.ChannelPaused,
]);

/** Types where an empty status is the ordinary state rather than a fault. */
const NEVER_RUNS: ReadonlySet<string> = new Set<string>([ChannelType.ChannelClientConnection]);

/** Whether this channel is carrying anything right now. */
export function running(channel: Channel): boolean {
  return channel.status === ChannelStatus.ChannelRunning;
}

/**
 * Whether a status is worth colouring.
 *
 * An empty status is not: a channel nobody has started has none, and so does
 * every client-connection definition. Reading that as a problem would paint
 * most of a fresh queue manager red.
 */
export function unhealthy(channel: Channel): boolean {
  if (channel.inDoubt) return true;
  return channel.status !== "" && UNHEALTHY.has(channel.status);
}

/** Whether an empty status on this channel means anything at all. */
export function statusExpected(channel: Channel): boolean {
  return !NEVER_RUNS.has(channel.type);
}

/**
 * The three groups a reader actually thinks in. It is not the type list: a
 * sender and a cluster sender differ in who wrote the definition, not in what
 * the channel does.
 */
export type ChannelGroup = "client" | "message" | "cluster";

export function group(channel: Channel): ChannelGroup {
  switch (channel.type) {
    case ChannelType.ChannelServerConnection:
    case ChannelType.ChannelClientConnection:
    case ChannelType.ChannelAMQP:
      return "client";
    case ChannelType.ChannelClusterSender:
    case ChannelType.ChannelClusterReceiver:
      return "cluster";
    default:
      return "message";
  }
}

/** UnknownMetric is -1 on the wire, and it means "nothing ever ran". */
export function metric(value: number): number | null {
  return value < 0 ? null : value;
}
