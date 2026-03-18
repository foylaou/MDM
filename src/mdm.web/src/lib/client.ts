import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { AuthService } from "../gen/mdm/v1/auth_connect";
import { DeviceService } from "../gen/mdm/v1/device_connect";
import { CommandService } from "../gen/mdm/v1/command_connect";
import { EventService } from "../gen/mdm/v1/event_connect";
import { VPPService } from "../gen/mdm/v1/vpp_connect";
import { UserService } from "../gen/mdm/v1/user_connect";
import { AuditService } from "../gen/mdm/v1/audit_connect";

const baseUrl = import.meta.env.DEV ? "" : window.location.origin;

function createTransport() {
  return createConnectTransport({
    baseUrl,
    credentials: "include", // send HttpOnly cookies
  });
}

export function createAuthClient() {
  return createPromiseClient(AuthService, createTransport());
}

export function createClients() {
  const transport = createTransport();
  return {
    auth: createPromiseClient(AuthService, transport),
    device: createPromiseClient(DeviceService, transport),
    command: createPromiseClient(CommandService, transport),
    event: createPromiseClient(EventService, transport),
    vpp: createPromiseClient(VPPService, transport),
    user: createPromiseClient(UserService, transport),
    audit: createPromiseClient(AuditService, transport),
  };
}

export type Clients = ReturnType<typeof createClients>;
