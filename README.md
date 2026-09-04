# FreePBX / FOP2 LDAP Directory

A lightweight LDAP server providing a searchable directory from the
**FOP2 Visual Phonebook** used with FreePBX/Asterisk.

This project is a modified fork of `affordablemobiles/freepbx-ldap`,
adapted to use the FOP2 Visual Phonebook as its contact source and to
improve compatibility with different IP phones and phone number formats.

## Features

-   Uses the **FOP2 Visual Phonebook** as a single source of contacts.
-   Provides LDAP directory searches without the need to synchronize or
    export contacts.
-   Supports searches by name and telephone number.
-   Handles national and international phone number formats such as:
    -   `071XXXXXXX`
    -   `+3371XXXXXXX`
    -   `003371XXXXXXX`
-   Uses `phonenumbers` for international phone number parsing and
    normalization.
-   Supports configurable MySQL connection settings.
-   Supports configurable LDAP listening port.
-   Configuration changes do **not** require recompiling the program.
-   Supports IPv4 and IPv6 MySQL server addresses.
-   Optional verbose debug logging.
-   Designed for use with Grandstream, Snom and other LDAP-capable IP
    phones.

------------------------------------------------------------------------

## How it works

The application starts a lightweight LDAP service on the port configured
by `LDAP_PORT`. The default port is:

``` text
10389
```

LDAP directory search requests from an IP phone are translated into SQL
queries against the FOP2 Visual Phonebook tables in the FreePBX/Asterisk
MySQL database.

The relevant FOP2 tables are:

``` text
visual_phonebook
visual_phonebook_phones
```

The LDAP attributes returned to the phone are:

``` text
displayName
telephoneNumber
```

The MySQL to LDAP mapping is:

``` text
lastname + firstname  -> displayName
number                -> telephoneNumber
```

Because the contacts are read directly from the FOP2 Visual Phonebook,
the LDAP directory is always up-to-date. There is no separate contact
import, export or synchronization process.

### LDAP authentication

Many IP phones require an LDAP bind with a username and password before
performing a directory search.

This server intentionally accepts any LDAP bind request and returns
success without validating the supplied username or password.

The LDAP credentials configured in the phone therefore do not provide
access control.

For this reason, the LDAP service should only be exposed to trusted
networks or protected by appropriate firewall rules.

------------------------------------------------------------------------

## Phone number searches

Telephone numbers may be stored or entered in different formats.

For example, the same French number may appear as:

``` text
071XXXXXXX
+3371XXXXXXX
003371XXXXXXX
```

The server normalizes telephone number searches to improve matching
between national and international representations.

International calling codes are handled using:

``` text
github.com/nyaruka/phonenumbers/v2
```

This avoids hard-coding individual country codes and also supports
international numbering plans including overseas territories.

Partial number searches are supported as well.

------------------------------------------------------------------------

## Configuration

The server reads its configuration from:

``` text
/opt/freepbx-ldap/freepbx-ldap.conf
```

Example:

``` ini
FREEPBX_SQLSERVER=localhost
FREEPBX_SQLUSER=freepbxldap
FREEPBX_SQLPASSWORD=secret
FREEPBX_SQLDATABASE=asterisk

LDAP_PORT=10389
DEBUG=OFF
```

Environment variables with the same names override values from the
configuration file.

### MySQL server

A hostname or IPv4 address may be specified with or without a port:

``` ini
FREEPBX_SQLSERVER=localhost
```

or:

``` ini
FREEPBX_SQLSERVER=192.168.111.9
```

A non-standard MySQL port can be specified explicitly:

``` ini
FREEPBX_SQLSERVER=192.168.111.9:3307
```

If no port is specified, MySQL port `3306` is used.

### IPv6

IPv6 addresses must be enclosed in square brackets:

``` ini
FREEPBX_SQLSERVER=[2001:db8::2]
```

With an explicit MySQL port:

``` ini
FREEPBX_SQLSERVER=[2001:db8::2]:3307
```

If no port is given, port `3306` is used.

### LDAP port

The LDAP listening port can be changed with:

``` ini
LDAP_PORT=10389
```

The default is `10389`.

Remember to update the LDAP port configured on the phones if this value
is changed.

### Debug logging

Verbose application logging can be enabled with:

``` ini
DEBUG=ON
```

For normal operation as a systemd service:

``` ini
DEBUG=OFF
```

The following values enable debug mode:

``` text
ON
TRUE
YES
1
```

Values are case-insensitive.

With debug mode enabled, additional information such as LDAP search
filters, generated SQL queries and returned directory entries is written
to the log.

`DEBUG=OFF` is recommended for normal operation as a systemd service.

------------------------------------------------------------------------

## Build

The Go runtime is required to build the application.

Clone the repository and run:

``` bash
go build
```

The required Go modules, including the phone number parsing library, are
managed through the Go module configuration.

------------------------------------------------------------------------

## Recommended installation

Create the application directory:

``` bash
mkdir -p /opt/freepbx-ldap
```

Copy the compiled binary:

``` bash
cp <freepbx-ldap binary location> /opt/freepbx-ldap/freepbx-ldap
```

Copy or create the configuration file:

``` bash
cp <freepbx-ldap.conf location> /opt/freepbx-ldap/freepbx-ldap.conf
```

Set ownership:

``` bash
chown -R asterisk:asterisk /opt/freepbx-ldap
```

Make the binary executable:

``` bash
chmod +x /opt/freepbx-ldap/freepbx-ldap
```

Protect the configuration file because it contains the MySQL
credentials:

``` bash
chmod 600 /opt/freepbx-ldap/freepbx-ldap.conf
```

Install the systemd service:

``` bash
cp <systemd/freepbx-ldap.service location> /etc/systemd/system/freepbx-ldap.service
```

Reload systemd:

``` bash
systemctl daemon-reload
```

Enable the service at boot:

``` bash
systemctl enable freepbx-ldap
```

Start it:

``` bash
systemctl start freepbx-ldap
```

Check its status:

``` bash
systemctl status freepbx-ldap
```

Logs can be viewed with:

``` bash
journalctl -u freepbx-ldap
```

For live logging:

``` bash
journalctl -fu freepbx-ldap
```

------------------------------------------------------------------------

# Phone Configuration

IP phones must be configured to use the FreePBX/FOP2 LDAP server.

The examples below provide working starting points for several phone
families.

## Grandstream GXP1620 / GXP1625

Grandstream phones should use **LDAP version 3**.

This is particularly important after a factory reset, as some firmware
versions may revert to LDAP version 2. LDAPv2 may result in the phone
connecting to the server and immediately closing the connection after
the bind.

Partial example from a Grandstream GXP1620 configuration:

``` text
# Generated by GXP1620
boot 1.0.7.4; core 1.0.7.9; base 1.0.7.23; prog 1.0.7.64

[...]

P8020=192.168.111.9
P8021=10389
P8022=dc=asterisk
P8023=asterisk
P8025=(telephoneNumber=%)
P8026=(|(displayName=%)(telephoneNumber=%))
P8028=displayName
P8029=telephoneNumber
P8030=%displayName  %telephoneNumber
P8031=100
P8032=15
P8033=1
P8034=1
P8035=1
P8036=displayName
P8037=0

[...]

# End of exported configuration
```

The LDAP server address and port must of course be adapted to the local
installation.

------------------------------------------------------------------------

## Snom 720 --- `snom720-main.htm`

``` xml
<?xml version="1.0" encoding="utf-8"?>
<settings>
    <phone-settings>
        *** Other Settings ***

        <ldap_server perm="">***server_ip***</ldap_server>
        <ldap_port perm="">10389</ldap_port>
        <ldap_base perm="">dc=asterisk</ldap_base>
        <ldap_username perm="">asterisk</ldap_username>
        <ldap_max_hits perm="">100</ldap_max_hits>
        <ldap_search_filter perm="">(&(telephoneNumber=*)(displayName=%))</ldap_search_filter>
        <ldap_number_filter perm="">(&(telephoneNumber=%)(displayName=*))</ldap_number_filter>
        <ldap_name_attributes perm="">displayName</ldap_name_attributes>
        <ldap_number_attributes perm="">telephoneNumber</ldap_number_attributes>
        <ldap_display_name perm="">%displayName</ldap_display_name>

        <gui_fkey1 perm="">keyevent F_DIRECTORY_SEARCH</gui_fkey1>
    </phone-settings>
</settings>
```

------------------------------------------------------------------------

## Snom 300 --- `snom300-main.htm`

``` xml
<?xml version="1.0" encoding="utf-8"?>
<settings>
    <phone-settings>
        *** Other Settings ***

        <ldap_server perm="">***server_ip***</ldap_server>
        <ldap_port perm="">10389</ldap_port>
        <ldap_base perm="">dc=asterisk</ldap_base>
        <ldap_username perm="">asterisk</ldap_username>
        <ldap_max_hits perm="">25</ldap_max_hits>
        <ldap_search_filter perm="">(&(telephoneNumber=*)(displayName=%))</ldap_search_filter>
        <ldap_number_filter perm="">(&(telephoneNumber=%)(displayName=*))</ldap_number_filter>
        <ldap_name_attributes perm="">displayName</ldap_name_attributes>
        <ldap_number_attributes perm="">telephoneNumber</ldap_number_attributes>
        <ldap_display_name perm="">%displayName</ldap_display_name>

        <idle_cancel_key_action perm="">keyevent F_DIRECTORY_SEARCH</idle_cancel_key_action>
    </phone-settings>

    <functionKeys e="2">
        <fkey idx="3" context="active" label="" perm="">keyevent F_DIRECTORY_SEARCH</fkey>
    </functionKeys>
</settings>
```

------------------------------------------------------------------------

## Polycom SoundPoint IP --- `sip.cfg`

Firmware UC 4 or newer is required.

``` xml
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<localcfg>
    *** Other Settings ***

    <dir>
        <dir.corp
            dir.corp.address="ldap://***server_ip***"
            dir.corp.port="10389"
            dir.corp.transport="TCP"
            dir.corp.baseDN="dc=asterisk"
            dir.corp.scope="sub"
            dir.corp.filterPrefix=""
            dir.corp.user="uid=asterisk,dc=asterisk"
            dir.corp.pageSize="32"
            dir.corp.password="supersecret"
            dir.corp.cacheSize="128"
            dir.corp.leg.pageSize="8"
            dir.corp.leg.cacheSize="32"
            dir.corp.autoQuerySubmitTimeout="1"
            dir.corp.viewPersistence="0"
            dir.corp.leg.viewPersistence="0"
            dir.corp.sortControl="0">

            <dir.corp.attribute
                dir.corp.attribute.1.name="displayName"
                dir.corp.attribute.1.label="Display Name"
                dir.corp.attribute.1.type="first_name"
                dir.corp.attribute.1.searchable="1"
                dir.corp.attribute.1.filter=""
                dir.corp.attribute.1.sticky="0"
                dir.corp.attribute.2.name="telephoneNumber"
                dir.corp.attribute.2.label="phone number"
                dir.corp.attribute.2.type="phone_number"
                dir.corp.attribute.2.filter=""
                dir.corp.attribute.2.sticky="0"
                dir.corp.attribute.2.searchable="1">
            </dir.corp.attribute>

            <dir.corp.backGroundSync
                dir.corp.backGroundSync.period="3600">
            </dir.corp.backGroundSync>

            <dir.corp.vlv
                dir.corp.vlv.allow="1"
                dir.corp.vlv.sortOrder="displayName telephoneNumber">
            </dir.corp.vlv>
        </dir.corp>
    </dir>

    <feature feature.corporateDirectory.enabled="1"/>
    <softkey softkey.feature.directories="1"/>
</localcfg>
```

------------------------------------------------------------------------

## Security

This LDAP server does **not** authenticate LDAP users.

Any username and password supplied in an LDAP bind request are accepted.

The service should therefore only be accessible from trusted local
networks or explicitly permitted hosts. Do not expose the LDAP port
directly to the public Internet.

The MySQL configuration file contains database credentials and should be
protected accordingly:

``` bash
chmod 600 /opt/freepbx-ldap/freepbx-ldap.conf
```

------------------------------------------------------------------------

## Troubleshooting

Run the application directly with:

``` ini
DEBUG=ON
```

to obtain verbose logging while testing LDAP searches.

Typical useful information includes:

``` text
Request BaseDn=...
Request FilterString=...
Request Attributes=...
Query SQL: ...
```

To verify that the LDAP port is listening:

``` bash
ss -lntp | grep 10389
```

A basic LDAP query can be tested with:

``` bash
ldapsearch \
    -H ldap://127.0.0.1:10389 \
    -D "asterisk" \
    -w "0000" \
    -b "dc=asterisk"
```

The username and password used for this test are arbitrary because LDAP
bind authentication is intentionally not enforced.

For Grandstream phones, verify that **LDAP version 3** is enabled if the
phone connects, sends a bind request and then immediately disconnects
without performing a search.
